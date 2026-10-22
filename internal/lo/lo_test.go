package lo

import (
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
)

func TestRefreshUsesSingleIsolatedSecureProcess(t *testing.T) {
	cases := []struct {
		name, sourceExt, outputExt, filter string
		wholeSheets                        bool
	}{
		{name: "XLSX recalculation", sourceExt: ".xlsx", outputExt: ".xlsx", filter: "Calc MS Excel 2007 XML"},
		{name: "XLSX to PDF export", sourceExt: ".xlsx", outputExt: ".pdf", filter: "calc_pdf_Export"},
		{name: "whole-sheet XLSX preview", sourceExt: ".xlsx", outputExt: ".pdf", filter: "calc_pdf_Export", wholeSheets: true},
		{name: "explicit DOCX refresh", sourceExt: ".docx", outputExt: ".docx", filter: "Office Open XML Text"},
		{name: "explicit PPTX refresh", sourceExt: ".pptx", outputExt: ".pptx", filter: "Impress MS PowerPoint 2007 XML"},
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
				if !strings.Contains(cmd.Args[1], "-env:UserInstallation=file:") {
					t.Fatal(cmd.Args)
				}
				if cmd.Args[len(cmd.Args)-1] != "macro:///Standard.Module1.Main" || strings.Contains(strings.Join(cmd.Args, " "), "--terminate_after_init") {
					t.Fatal("unexpected LibreOffice invocation", cmd.Args)
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
	err = Refresh(context.WithValue(context.Background(), commandRunnerKey{}, runner), src, dst, false)
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

// Command-only environment isolation, independent of the Refresh macro path.
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
