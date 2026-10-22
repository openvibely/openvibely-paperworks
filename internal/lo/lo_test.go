package lo

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/testdeps"
	"github.com/xuri/excelize/v2"
)

func TestPlainXLSXRefreshCopiesWithoutLibreOffice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.xlsx")
	dst := filepath.Join(dir, "refreshed.xlsx")
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "Category"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A2", "Apples"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "B2", 42); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(src); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if xlsxHasRefreshWork(src) {
		t.Fatal("plain workbook was classified as needing a LibreOffice refresh")
	}
	formulaPath := filepath.Join(dir, "formula.xlsx")
	formulaBook := excelize.NewFile()
	if err := formulaBook.SetCellFormula("Sheet1", "A1", "1+1"); err != nil {
		t.Fatal(err)
	}
	if err := formulaBook.SaveAs(formulaPath); err != nil {
		t.Fatal(err)
	}
	if err := formulaBook.Close(); err != nil {
		t.Fatal(err)
	}
	if !xlsxHasRefreshWork(formulaPath) {
		t.Fatal("formula workbook was incorrectly classified as a no-op")
	}
	chartSrc := "../../testdata/fixtures/reference-pie.xlsx"
	if xlsxHasRefreshWork(chartSrc) {
		t.Fatal("chart-only workbook should be eligible for local cache refresh")
	}
	// A missing executable proves these safe refreshes do not launch Office.
	t.Setenv("DOCUMENT_SOFFICE", filepath.Join(dir, "missing-soffice"))
	if err := Refresh(context.Background(), src, dst, false); err != nil {
		t.Fatal(err)
	}
	refreshed, err := os.ReadFile(dst)
	if err != nil || !bytes.Equal(refreshed, original) {
		t.Fatal("no-op refresh did not preserve the workbook byte-for-byte", err)
	}
	chartBefore, err := os.ReadFile(chartSrc)
	if err != nil {
		t.Fatal(err)
	}
	chartDst := filepath.Join(dir, "chart-refreshed.xlsx")
	if err := Refresh(context.Background(), chartSrc, chartDst, false); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(chartDst)
	if err != nil {
		t.Fatal(err)
	}
	chartFound := false
	for _, part := range archive.File {
		if strings.Contains(part.Name, "/charts/") {
			chartFound = true
			break
		}
	}
	archive.Close()
	if !chartFound {
		t.Fatal("locally refreshed workbook lost its chart cache parts")
	}
	unchanged, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("source changed", err)
	}
	chartUnchanged, err := os.ReadFile(chartSrc)
	if err != nil || !bytes.Equal(chartUnchanged, chartBefore) {
		t.Fatal("source chart workbook changed", err)
	}
}

func TestLibreOfficeXLSXPDFAndWholeSheetPreview(t *testing.T) {
	if err := testdeps.Office(Find); err != nil {
		t.Skip("LibreOffice unavailable: " + err.Error())
	}
	src := "../../testdata/fixtures/reference-pie.xlsx"
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		wholeSheets bool
	}{
		{name: "XLSX PDF export"},
		{name: "whole-sheet XLSX preview", wholeSheets: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "output.pdf")
			launches := 0
			runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
				launches++
				return cmd.CombinedOutput()
			})
			ctx := context.WithValue(context.Background(), commandRunnerKey{}, runner)
			if err := Refresh(ctx, src, dst, tc.wholeSheets); err != nil {
				t.Fatal(err)
			}
			if launches != 1 {
				t.Fatalf("LibreOffice launches = %d, want 1", launches)
			}
			data, err := os.ReadFile(dst)
			if err != nil || len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
				t.Fatalf("PDF output missing or invalid: %v", err)
			}
		})
	}
	unchanged, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("source workbook changed", err)
	}
}

func TestLibreOfficeExplicitWordRefresh(t *testing.T) {
	if err := testdeps.Office(Find); err != nil {
		t.Skip("LibreOffice unavailable: " + err.Error())
	}
	src := "../../testdata/inspection-reports/call-0344.docx"
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "refreshed.docx")
	launches := 0
	runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
		launches++
		return cmd.CombinedOutput()
	})
	ctx := context.WithValue(context.Background(), commandRunnerKey{}, runner)
	if err := Refresh(ctx, src, dst, false); err != nil {
		t.Fatal(err)
	}
	if launches != 1 {
		t.Fatalf("LibreOffice launches = %d, want 1", launches)
	}
	archive, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	foundDocument := false
	for _, file := range archive.File {
		if file.Name == "word/document.xml" {
			foundDocument = true
			break
		}
	}
	if !foundDocument {
		t.Fatal("refreshed Word document package is incomplete")
	}
	unchanged, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("source Word document changed", err)
	}
}

func TestRefreshUsesSingleIsolatedSecureProcess(t *testing.T) {
	cases := []struct {
		name, sourceExt, outputExt, filter string
		wholeSheets, usesMacro             bool
	}{
		{name: "XLSX recalculation", sourceExt: ".xlsx", outputExt: ".xlsx", filter: "Calc MS Excel 2007 XML", usesMacro: true},
		{name: "XLSX to PDF export", sourceExt: ".xlsx", outputExt: ".pdf", filter: "calc_pdf_Export", usesMacro: true}, {name: "whole-sheet XLSX preview", sourceExt: ".xlsx", outputExt: ".pdf", filter: "calc_pdf_Export", wholeSheets: true, usesMacro: true},
		{name: "explicit DOCX refresh", sourceExt: ".docx", outputExt: ".docx", filter: "Office Open XML Text", usesMacro: true},
		{name: "explicit PPTX refresh", sourceExt: ".pptx", outputExt: ".pptx", filter: "Impress MS PowerPoint 2007 XML", usesMacro: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("DOCUMENT_SOFFICE", exe)
			originalEnv := os.Environ()
			dir := t.TempDir()
			src := filepath.Join(dir, "input"+tc.sourceExt)
			dst := filepath.Join(dir, "output"+tc.outputExt)
			original := []byte("unchanged original input")
			if err := os.WriteFile(src, original, 0600); err != nil {
				t.Fatal(err)
			}
			var profile string
			calls := 0
			runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
				calls++
				if len(cmd.Args) < 2 || !strings.Contains(cmd.Args[1], "-env:UserInstallation=file:") {
					t.Fatal(cmd.Args)
				}
				if strings.Contains(strings.Join(cmd.Args, " "), "--terminate_after_init") {
					t.Fatal("unexpected profile initialization launch", cmd.Args)
				}
				u, err := url.Parse(strings.TrimPrefix(cmd.Args[1], "-env:UserInstallation="))
				if err != nil {
					t.Fatal(err)
				}
				profile = u.Path
				config, err := os.ReadFile(filepath.Join(profile, "user", "registrymodifications.xcu"))
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"MacroSecurityLevel", "DisableMacrosExecution", "<value>true</value>"} {
					if !bytes.Contains(config, []byte(want)) {
						t.Fatal("security configuration missing", want)
					}
				}
				env := map[string]string{}
				for _, v := range cmd.Env {
					k, v, ok := strings.Cut(v, "=")
					if ok {
						env[k] = v
					}
				}
				if env["HOME"] != os.Getenv("HOME") {
					t.Fatal("HOME changed")
				}
				for _, k := range []string{"XDG_CACHE_HOME", "XDG_CONFIG_HOME"} {
					st, e := os.Stat(env[k])
					if e != nil || !st.IsDir() || env[k] == os.Getenv(k) {
						t.Fatal(k, e)
					}
				}
				if tc.usesMacro {
					if cmd.Args[len(cmd.Args)-1] != "macro:///Standard.Module1.Main" {
						t.Fatal("unexpected macro invocation", cmd.Args)
					}
					lib := filepath.Join(profile, "user", "basic")
					index, err := os.ReadFile(filepath.Join(lib, "Standard", "script.xlb"))
					if err != nil || !bytes.Contains(index, []byte(`library:name="Module1"`)) {
						t.Fatal("Basic library index missing Module1", err, string(index))
					}
					list, err := os.ReadFile(filepath.Join(lib, "script.xlc"))
					if err != nil || !bytes.Contains(list, []byte(`library:name="Standard"`)) || !bytes.Contains(list, []byte(`library:link="false"`)) {
						t.Fatal("profile Basic library not registered", err, string(list))
					}
					macro, err := os.ReadFile(filepath.Join(lib, "Standard", "Module1.xba"))
					if err != nil || !bytes.Contains(macro, []byte("calculateAll")) || !bytes.Contains(macro, []byte("MacroExecutionMode")) || !bytes.Contains(macro, []byte("NEVER_EXECUTE")) {
						t.Fatal("secure refresh macro unavailable", err)
					}
					if env["PW_IN"] != FileURL(src) || env["PW_OUT"] != FileURL(dst) || env["PW_FILTER"] != tc.filter {
						t.Fatal("refresh environment mismatch", env)
					}
					wantWhole := ""
					if tc.wholeSheets {
						wantWhole = "1"
					}
					if env["PW_WHOLE_SHEETS"] != wantWhole {
						t.Fatal("whole-sheet flag mismatch", env["PW_WHOLE_SHEETS"])
					}
					if err := os.WriteFile(dst, []byte("successful refreshed output"), 0600); err != nil {
						t.Fatal(err)
					}
					return nil, os.WriteFile(env["PW_STATUS"], []byte("ok"), 0600)
				}
				t.Fatal("unexpected non-macro refresh", cmd.Args)
				return nil, errors.New("unexpected non-macro refresh")
			})
			ctx := context.WithValue(context.Background(), commandRunnerKey{}, runner)
			if err := Refresh(ctx, src, dst, tc.wholeSheets); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("LibreOffice launches = %d, want 1", calls)
			}
			if _, err := os.Stat(profile); !os.IsNotExist(err) {
				t.Fatal("temporary profile not removed", err)
			}
			if !reflect.DeepEqual(os.Environ(), originalEnv) {
				t.Fatal("process environment changed")
			}
			data, err := os.ReadFile(src)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatal("source changed", err)
			}
			data, err = os.ReadFile(dst)
			if err != nil || len(data) == 0 {
				t.Fatal("successful output missing or empty", err)
			}
		})
	}
}

func TestFailedOfficeProcessCannotPublishSuccess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCUMENT_SOFFICE", exe)
	dir := t.TempDir()
	src := filepath.Join(dir, "input.xlsx")
	dst := filepath.Join(dir, "output.xlsx")
	original := []byte("original workbook")
	if err := os.WriteFile(src, original, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		if cmd.Args[len(cmd.Args)-1] != "macro:///Standard.Module1.Main" {
			t.Fatal("unexpected process invocation", cmd.Args)
		}
		for _, v := range cmd.Env {
			if strings.HasPrefix(v, "PW_STATUS=") {
				if e := os.WriteFile(strings.TrimPrefix(v, "PW_STATUS="), []byte("ok"), 0600); e != nil {
					t.Fatal(e)
				}
			}
		}
		if e := os.WriteFile(dst, []byte("partial"), 0600); e != nil {
			t.Fatal(e)
		}
		return []byte("failed after output creation"), errors.New("process failed")
	})
	err = Refresh(context.WithValue(context.Background(), commandRunnerKey{}, runner), src, dst, true)
	if err == nil || !strings.Contains(err.Error(), "process failed") {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("LibreOffice launches = %d, want 1", calls)
	}
	if _, err = os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("failed output published", err)
	}
	data, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("source changed", err)
	}
}

func TestReferenceCommandEnvironmentIsolation(t *testing.T) {
	environment := func(values []string) map[string]string {
		result := make(map[string]string)
		for _, value := range values {
			if key, value, ok := strings.Cut(value, "="); ok {
				result[key] = value
			}
		}
		return result
	}
	original := environment(os.Environ())
	root := t.TempDir()
	// Reference mocks executable lookup; the runner below intercepts execution.
	s := &session{exe: "/fixture/soffice", tmp: root, profile: filepath.Join(root, "profile")}
	calls := 0
	runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		env := environment(cmd.Env)
		home, present := env["HOME"]
		oldHome, oldPresent := original["HOME"]
		if home != oldHome || present != oldPresent {
			t.Fatal("HOME changed")
		}
		for _, key := range []string{"XDG_CACHE_HOME", "XDG_CONFIG_HOME"} {
			info, err := os.Stat(env[key])
			if err != nil || !info.IsDir() {
				t.Fatal(key, err)
			}
		}
		if cache, present := original["XDG_CACHE_HOME"]; present && env["XDG_CACHE_HOME"] == cache {
			t.Fatal("cache directory not isolated")
		}
		return nil, nil
	})
	ctx := context.WithValue(context.Background(), commandRunnerKey{}, runner)
	if err := s.run(ctx, nil, "--headless"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(environment(os.Environ()), original) {
		t.Fatal("process environment changed")
	}
	// Supplemental guard: absence of a callback cannot satisfy its assertions.
	if calls != 1 {
		t.Fatal("command callback count", calls)
	}
}
