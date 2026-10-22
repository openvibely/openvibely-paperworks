package convert

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func referencePackage(t *testing.T, name string, parts map[string]string) string {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, s := range parts {
		w, e := z.Create(n)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(s)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), name)
	if e := os.WriteFile(p, b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func referenceBook(t *testing.T, part, sheet, cell string, opaque bool) string {
	t.Helper()
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="` + office.NSS + `" xmlns:r="` + office.NSR + `"><sheets><sheet name="` + sheet + `" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="/xl/worksheets/` + part + `"/></Relationships>`,
		"xl/worksheets/" + part:      `<worksheet xmlns="` + office.NSS + `"><sheetData><row>` + cell + `</row></sheetData></worksheet>`,
	}
	if opaque {
		parts["opaque-custom-part.bin"] = "custom feature"
	}
	return referencePackage(t, "source.xlsx", parts)
}
func contents(t *testing.T, p string) []byte {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func withDeps(d dependencies) context.Context {
	return context.WithValue(context.Background(), dependenciesKey{}, d)
}
func assertUnchanged(t *testing.T, p string, b []byte) {
	t.Helper()
	if !bytes.Equal(contents(t, p), b) {
		t.Fatal("source changed")
	}
}

// Original DocumentChecks assertions: preserve sheet identity, formulas and
// opaque parts while merging caches from an engine-renumbered worksheet.
func TestReferenceRecalculationPreservation(t *testing.T) {
	for _, opaque := range []bool{false, true} {
		t.Run(map[bool]string{false: "sheet-identity", true: "opaque-parts"}[opaque], func(t *testing.T) {
			src := referenceBook(t, "sheet7.xml", "Forecast", `<c r="B2"><f>A2*2</f><v>4</v></c>`, opaque)
			converted := referenceBook(t, "sheet1.xml", "Forecast", `<c r="B2"><f>A2*2</f><v>4</v></c>`, false)
			if opaque {
				renamed := filepath.Join(filepath.Dir(src), "preserve.xlsx")
				if err := os.Rename(src, renamed); err != nil {
					t.Fatal(err)
				}
				src = renamed
			}
			before := contents(t, src)
			ctx := withDeps(dependencies{refresh: func(_ context.Context, in, out string, _ bool) error {
				if in != src {
					t.Fatal(in)
				}
				return os.WriteFile(out, contents(t, converted), 0600)
			}})
			report, err := Convert(ctx, src, filepath.Join(t.TempDir(), "output"), Options{Recalculate: true})
			if err != nil {
				t.Fatal(err)
			}
			var fs []office.Formula
			if opaque {
				if report["original_package_preserved_except_calculated_results"] != true {
					t.Fatal(report)
				}
				z, err := zip.OpenReader(report["output"].(string))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := z.Close(); err != nil {
						t.Error(err)
					}
				})
				member, err := z.Open("opaque-custom-part.bin")
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(member)
				closeErr := member.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
				if string(data) != "custom feature" {
					t.Fatal("opaque part lost")
				}
				names := map[string]bool{}
				for _, f := range z.File {
					names[f.Name] = true
				}
				if !names["xl/worksheets/sheet7.xml"] {
					t.Fatal("original sheet missing")
				}
				if names["xl/worksheets/sheet1.xml"] {
					t.Fatal("engine sheet leaked")
				}
				fs = report["formulas"].([]office.Formula)
				if len(fs) == 0 || fs[0].Formula == nil || *fs[0].Formula != "A2*2" {
					t.Fatal(fs)
				}
				assertUnchanged(t, src, before)
			} else {
				if report["formula_cells_preserved"] != true {
					t.Fatal(report)
				}
				fs = report["formulas"].([]office.Formula)
				if len(fs) == 0 || fs[0].Sheet != "Forecast" {
					t.Fatal(fs)
				}
			}
			// Supplemental checks follow all original assertions.
			if report["formula_cells_preserved"] != true || report["original_package_preserved_except_calculated_results"] != true {
				t.Fatal(report)
			}
			if len(fs) != 1 || fs[0].Sheet != "Forecast" || fs[0].Formula == nil || *fs[0].Formula != "A2*2" {
				t.Fatal(fs)
			}
			p, err := opc.Open(report["output"].(string))
			if err != nil {
				t.Fatal(err)
			}
			if !p.Has("xl/worksheets/sheet7.xml") || p.Has("xl/worksheets/sheet1.xml") {
				t.Fatal(p.Names())
			}
			assertUnchanged(t, src, before)
		})
	}
}
func TestReferenceRecalculationRejectsLostFormulasAndRenamedSheets(t *testing.T) {
	src := referenceBook(t, "sheet1.xml", "Forecast", `<c r="B2"><f>A2*2</f><v>4</v></c>`, false)
	before := contents(t, src)
	for _, tc := range []struct{ name, sheet, cell string }{
		{"flattened", "Forecast", `<c r="B2"><v>4</v></c>`},
		{"renamed", "Other", `<c r="B2"><f>A2*2</f><v>4</v></c>`},
	} {
		t.Run(tc.name, func(t *testing.T) {

			converted := referenceBook(t, "sheet3.xml", tc.sheet, tc.cell, false)
			out := filepath.Join(t.TempDir(), "output")
			ctx := withDeps(dependencies{refresh: func(_ context.Context, in, dst string, _ bool) error {
				return os.WriteFile(dst, contents(t, converted), 0600)
			}})
			_, err := Convert(ctx, src, out, Options{Recalculate: true})
			if err == nil || !strings.Contains(err.Error(), "Recalculation") {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(out, "source.xlsx")); !os.IsNotExist(err) {
				t.Fatal("failed output retained", err)
			}
			assertUnchanged(t, src, before)
		})
	}
}
func TestReferenceConverterFailureAndNativeSensitivityTables(t *testing.T) {
	for index, formula := range []string{`<f t="dataTable" ref="E3:F4" dt2D="1" r1="B1" r2="B2"/>`, `<f>TABLE(D2,B1,E2,B2,D3)</f>`} {
		src := referencePackage(t, fmt.Sprintf("native-%d.xlsx", index), map[string]string{"xl/workbook.xml": `<workbook xmlns="` + office.NSS + `"/>`, "xl/worksheets/sheet1.xml": `<worksheet xmlns="` + office.NSS + `"><sheetData><row><c r="E3">` + formula + `<v>103</v></c></row></sheetData></worksheet>`})
		before := contents(t, src)
		out := filepath.Join(t.TempDir(), "output")
		refreshCalls, exportCalls := 0, 0
		failure := errors.New("calculation service failed")
		ctx := withDeps(dependencies{refresh: func(context.Context, string, string, bool) error {
			refreshCalls++
			return failure
		}, export: func(context.Context, string, string, string) (string, error) {
			exportCalls++
			return "", nil
		}})
		_, err := Convert(ctx, src, out, Options{Recalculate: true})
		// Preserve Reference's order: expected exception, no plain command, one refresh.
		if !errors.Is(err, failure) || !strings.Contains(err.Error(), "calculation service failed") {
			t.Fatal("expected calculation service failure", err)
		}
		if exportCalls != 0 {
			t.Fatal("plain converter calls", exportCalls)
		}
		if refreshCalls != 1 {
			t.Fatal("refresh calls", refreshCalls)
		}
		if _, err = os.Stat(filepath.Join(out, filepath.Base(src))); !os.IsNotExist(err) {
			t.Fatal("failed output retained", err)
		}
		assertUnchanged(t, src, before)
	}
}

// These are separate original methods; neither inherits the other's source or output.
func TestReferenceConverterNoOutput(t *testing.T) {
	src := referencePackage(t, "input.xlsx", map[string]string{"xl/workbook.xml": "<workbook/>"})
	ctx := withDeps(dependencies{refresh: func(context.Context, string, string, bool) error { return nil }})
	_, err := Convert(ctx, src, filepath.Join(t.TempDir(), "empty"), Options{Recalculate: true})
	if err == nil || !strings.Contains(err.Error(), "no output") {
		t.Fatal(err)
	}
}
func TestReferenceConverterExistingEmptyDirectory(t *testing.T) {
	src := referencePackage(t, "input.xlsx", map[string]string{"xl/workbook.xml": "<workbook/>"})
	before := contents(t, src)
	out := filepath.Join(t.TempDir(), "out")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := Convert(context.Background(), src, out, Options{Recalculate: true})
	if !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	assertUnchanged(t, src, before)
}

// Supplemental existing-file and call-count regression retained independently.
func TestReferenceConverterNoOutputAndExistingOutput(t *testing.T) {
	src := referencePackage(t, "input.xlsx", map[string]string{"xl/workbook.xml": "<workbook/>"})
	before := contents(t, src)
	calls := 0
	ctx := withDeps(dependencies{refresh: func(context.Context, string, string, bool) error { calls++; return nil }})
	_, err := Convert(ctx, src, filepath.Join(t.TempDir(), "empty"), Options{Recalculate: true})
	if err == nil || !strings.Contains(err.Error(), "no output") || calls != 1 {
		t.Fatal(err, calls)
	}
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "input.xlsx")
	os.WriteFile(sentinel, []byte("stale"), 0600)
	_, err = Convert(ctx, src, dir, Options{Recalculate: true})
	if !errors.Is(err, os.ErrExist) || calls != 1 {
		t.Fatal(err, calls)
	}
	if string(contents(t, sentinel)) != "stale" {
		t.Fatal("stale output overwritten")
	}
	assertUnchanged(t, src, before)
}
func TestReferenceRenderForwardsSelection(t *testing.T) {
	root := t.TempDir()
	src, out, pdfPath := filepath.Join(root, "source.xlsx"), filepath.Join(root, "render"), filepath.Join(root, "out.pdf")
	type call struct {
		in, out string
		options Options
	}
	var calls []call
	ctx := withDeps(dependencies{convert: func(_ context.Context, in, dst string, o Options) (map[string]any, error) {
		calls = append(calls, call{in, dst, o})
		return map[string]any{"output": pdfPath}, nil
	}, render: func(_ context.Context, in, dst string, o pdf.RenderOptions) ([]string, error) {
		if in != pdfPath || dst != out {
			t.Fatal(in, dst)
		}
		return []string{"page.png"}, nil
	}})
	got, err := RenderFile(ctx, src, out, "Inputs", "B2:D4", pdf.RenderOptions{MaxPixels: 1600})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"page.png"}) {
		t.Fatal(got)
	}
	if len(calls) != 1 {
		t.Fatal(calls)
	}
	c := calls[0]
	if c.in != src || c.out != out || !reflect.DeepEqual(c.options, Options{Sheet: "Inputs", CellRange: "B2:D4"}) {
		t.Fatal(c)
	}
}
func TestReferenceRendererRejectsUnsupportedChartBeforeRaster(t *testing.T) {
	ctx := withDeps(dependencies{convert: func(context.Context, string, string, Options) (map[string]any, error) {
		return map[string]any{"output": "../../testdata/fixtures/libreoffice-unsupported-treemap.pdf"}, nil
	}, render: func(context.Context, string, string, pdf.RenderOptions) ([]string, error) {
		t.Fatal("raster called after conversion failure")
		return nil, nil
	}})
	_, err := RenderFile(ctx, "../../testdata/fixtures/reference-treemap.pptx", filepath.Join(t.TempDir(), "preview"), "", "", pdf.RenderOptions{MaxPixels: 1600})
	if err == nil || !strings.Contains(err.Error(), "could not render") {
		t.Fatal(err)
	}
}

// The original DocumentChecks.package iterates its insertion-ordered dict and
// uses ZIP_STORED. Preserve that scope here instead of ranging over a Go map.
func referenceProfilePackage(t *testing.T, name, worksheet string) string {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, part := range []struct{ name, text string }{
		{"xl/workbook.xml", "<workbook/>"},
		{"xl/worksheets/sheet1.xml", worksheet},
	} {
		w, err := z.CreateHeader(&zip.FileHeader{Name: part.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, part.text); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReferenceConversionInspectsDistinctArtifact(t *testing.T) {
	src := referenceProfilePackage(t, "input.xlsx", `<worksheet xmlns="`+office.NSS+`"><sheetData><row><c r="A1"><f>1+1</f></c></row></sheetData></worksheet>`)
	converted := referenceProfilePackage(t, "engine.xlsx", `<worksheet xmlns="`+office.NSS+`"><sheetData><row><c r="A1"><f>1+1</f><v>2</v></c></row></sheetData></worksheet>`)
	before := contents(t, src)
	out := filepath.Join(t.TempDir(), "converted")
	destination := filepath.Join(out, "input.xlsx")
	// Exercise Convert -> lo.Refresh -> real isolated profile creation. Only
	// the external office process is replaced, as Reference replaces refresh_office.
	// The process checks the profile and paths before publishing the distinct ZIP.
	dir := t.TempDir()
	exe := filepath.Join(dir, "office-probe")
	marker := filepath.Join(dir, "calls")
	script := `#!/bin/sh
set -eu
profile=${1#-env:UserInstallation=file://}
test -f "$profile/user/registrymodifications.xcu"
grep -q 'MacroSecurityLevel' "$profile/user/registrymodifications.xcu"
printf '%s\n' "$profile" >> "$PW_TEST_CALLS"
test -f "$profile/user/basic/script.xlc"
test -f "$profile/user/basic/Standard/script.xlb"
grep -q 'NEVER_EXECUTE' "$profile/user/basic/Standard/Module1.xba"
test "$PW_IN" = "$PW_TEST_INPUT"
test "$PW_OUT" = "$PW_TEST_OUTPUT"
test "$PW_IN" != "$PW_OUT"
cp "$PW_TEST_ARTIFACT" "$PW_TEST_DESTINATION"
printf ok > "$PW_STATUS"
`
	if err := os.WriteFile(exe, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCUMENT_SOFFICE", exe)
	t.Setenv("PW_TEST_CALLS", marker)
	t.Setenv("PW_TEST_INPUT", lo.FileURL(src))
	t.Setenv("PW_TEST_OUTPUT", lo.FileURL(destination))
	t.Setenv("PW_TEST_DESTINATION", destination)
	t.Setenv("PW_TEST_ARTIFACT", converted)
	report, err := Convert(context.Background(), src, out, Options{Recalculate: true})
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Fields(string(contents(t, marker)))
	if len(calls) != 1 {
		t.Fatal("refresh should launch LibreOffice once", calls)
	}
	if _, err := os.Stat(calls[0]); !os.IsNotExist(err) {
		t.Fatal("temporary profile retained", err)
	}
	fs := report["formulas"].([]office.Formula)
	if len(fs) != 1 || fs[0].Cached == nil || *fs[0].Cached != "2" || report["output"] != destination {
		t.Fatal(report)
	}
	assertUnchanged(t, src, before)
}

func TestReferenceDisposableHierarchyPreview(t *testing.T) {
	src := "../../testdata/fixtures/reference-hierarchy-treemap.pptx"
	before := contents(t, src)
	seen := ""
	ctx := withDeps(dependencies{export: func(ctx context.Context, in, out, target string) (string, error) {
		seen = in
		if in == src {
			t.Fatal("original sent to converter")
		}
		if referenceDisposableFirstSlideHasChart(t, in) {
			t.Fatal("chart not replaced in preview")
		}
		dst := filepath.Join(out, stem(src)+".pdf")
		// Captured from the unchanged original Canvas/drawString/save fixture.
		err := os.WriteFile(dst, contents(t, "../../testdata/reference-mirror/disposable-preview.pdf"), 0600)
		return dst, err
	}})
	pages, err := RenderFile(ctx, src, filepath.Join(t.TempDir(), "render"), "", "", pdf.RenderOptions{MaxPixels: 1600})
	if err != nil || len(pages) != 1 {
		t.Fatal(pages, err)
	}
	if seen == "" {
		t.Fatal("converter not called")
	}
	if _, err = os.Stat(seen); !os.IsNotExist(err) {
		t.Fatal("disposable copy retained", err)
	}
	assertUnchanged(t, src, before)
}

func TestReferenceLinkedChartInputCaches(t *testing.T) {
	for _, kind := range []string{"alternate", "hierarchy"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			src := "../../testdata/reference-mirror/linked-hierarchy.pptx"
			spec := ChartInputSpec{Chart: 1, Sheet: "Source", Cells: map[string]any{"A1": "West", "C1": float64(42)}}
			if kind == "alternate" {
				src = testsupport.PresentationOutput(t, root)
				if _, err := MaterializeCharts("../../testdata/fixtures/reference-bar.pptx", src); err != nil {
					t.Fatal(err)
				}
				p, err := opc.Open(src)
				if err != nil {
					t.Fatal(err)
				}
				verified, err := office.VerifyChartData(p, "")
				if err != nil || len(verified) == 0 {
					t.Fatal(verified, err)
				}
				part := verified[0].(map[string]any)["chart"].(string)
				d, err := p.XML(part)
				if err != nil {
					t.Fatal(err)
				}
				formula := office.Text(office.All(d.Root(), office.NSC, "numRef")[0], office.NSC, "f")
				sheet, reference, ok := strings.Cut(formula, "!")
				if !ok {
					t.Fatal(formula)
				}
				spec = ChartInputSpec{ChartPart: part, Sheet: strings.Trim(sheet, "'"), Cells: map[string]any{strings.ReplaceAll(strings.Split(reference, ":")[0], "$", ""): float64(42)}}
			}
			if kind == "hierarchy" {
				original, err := opc.Open(src)
				if err != nil {
					t.Fatal(err)
				}
				z, err := zip.OpenReader("../../testdata/fixtures/reference-hierarchy-treemap.pptx")
				if err != nil {
					t.Fatal(err)
				}
				defer z.Close()
				src, err = testsupport.MutateStoredZIP(z, func(parts *testsupport.OrderedZIP) error {
					// Build the editable XML view from the acquired callback dictionary,
					// not by reopening the source or copying a finished linked package.
					raw, err := testsupport.StoredZIPBytes(parts)
					if err != nil {
						return err
					}
					p, err := opc.Read(raw)
					if err != nil {
						return err
					}
					part := "ppt/slides/extendedCharts/chart.xml"
					d, err := p.XML(part)
					if err != nil {
						return err
					}
					data := office.Child(office.Child(d.Root(), office.NSCX, "chartData"), office.NSCX, "data")
					for _, entry := range []struct{ kind, formula string }{{"strDim", "'Source'!$A$1:$B$3"}, {"numDim", "'Source'!$C$1:$C$3"}} {
						dim := office.Child(data, office.NSCX, entry.kind)
						f := dim.CreateElement("cx:f")
						f.SetText(entry.formula)
						dim.RemoveChild(f)
						dim.InsertChildAt(0, f)
						if entry.kind == "numDim" {
							office.Child(dim, office.NSCX, "lvl").CreateAttr("formatCode", "0.00")
						}
					}
					ext := office.Child(d.Root(), office.NSCX, "chartData").CreateElement("cx:externalData")
					ext.CreateAttr("xmlns:r", office.NSR)
					ext.CreateAttr("r:id", "book")
					if err := p.SetXML(part, d); err != nil {
						return err
					}
					p.Set(opc.RelsName(part), []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="book" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/package" Target="../../embeddings/source.xlsx"/></Relationships>`))
					p.Set("ppt/embeddings/source.xlsx", original.Get("ppt/embeddings/source.xlsx"))
					if err := p.EnsureContentType("ppt/embeddings/source.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); err != nil {
						return err
					}
					for _, name := range p.Names() {
						parts.Set(name, p.Get(name))
					}
					return nil
				}, func() string { return testsupport.PresentationOutput(t, root) })
				if err != nil {
					t.Fatal(err)
				}
				part := "ppt/slides/extendedCharts/chart.xml"
				// Exercise the saved package boundary before calling ChartInputs.
				reopened, err := opc.Open(src)
				if err != nil || reopened.Get(part) == nil {
					t.Fatal("saved mutation", err)
				}
			}
			before := contents(t, src)
			calls := 0
			ctx := withDeps(dependencies{convert: func(_ context.Context, in, out string, o Options) (map[string]any, error) {
				calls++
				if !o.Recalculate {
					t.Fatal(o)
				}
				if err := os.Mkdir(out, 0700); err != nil {
					return nil, err
				}
				dst := filepath.Join(out, "calculated.xlsx")
				err := os.WriteFile(dst, contents(t, in), 0600)
				return map[string]any{"output": dst}, err
			}})
			out := testsupport.PresentationOutput(t, root)
			report, err := ChartInputs(ctx, src, out, spec)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal(calls)
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			// The original alternate case reads the same report-selected part
			// after editing, rather than rediscovering a possibly different chart.
			part := spec.ChartPart
			if kind != "alternate" {
				// Reference reads the edited report's selected chart, not fresh discovery.
				part = report["charts"].([]any)[0].(*office.ChartExReport).Chart
			}
			d, err := p.XML(part)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "alternate" {
				verified, err := office.VerifyChartData(p, "")
				if err != nil || len(verified) != 1 {
					t.Fatal("verified chart report", verified, err)
				}
				value := office.Text(office.Child(office.Child(office.All(d.Root(), office.NSC, "numRef")[0], office.NSC, "numCache"), office.NSC, "pt"), office.NSC, "v")
				number, err := strconv.ParseFloat(value, 64)
				if err != nil || number != 42 {
					t.Fatal(value, err)
				}
			} else {
				// Inspect the original reference assertions via JSON to retain their types.
				raw, err := json.Marshal(report)
				if err != nil {
					t.Fatal(err)
				}
				var r map[string]any
				if err = json.Unmarshal(raw, &r); err != nil {
					t.Fatal(err)
				}
				chart := r["charts"].([]any)[0].(map[string]any)
				dims := chart["series"].([]any)[0].(map[string]any)["dimensions"].(map[string]any)
				if !reflect.DeepEqual(dims["size"], []any{float64(42), float64(20), float64(5)}) || !reflect.DeepEqual(dims["category_paths"].([]any)[0], []any{"West", "A"}) {
					t.Fatal(dims)
				}
				changed := report["changed_parts"].([]string)
				if !reflect.DeepEqual(changed, []string{"ppt/embeddings/source.xlsx", "ppt/slides/extendedCharts/chart.xml"}) {
					t.Fatal(changed)
				}
				if office.Child(office.All(d.Root(), office.NSCX, "numDim")[0], office.NSCX, "lvl").SelectAttrValue("formatCode", "") != "0.00" {
					t.Fatal("format code lost")
				}
			}
			assertUnchanged(t, src, before)
		})
	}
}

func TestReferenceSparklineRecalculatesBeforePreview(t *testing.T) {
	input := referenceCallbackInput(t, "stale.xlsx")
	pkg, err := opc.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := office.EditSparklines(pkg, office.SparklineSpec{Sheet: "Inputs", Sparklines: []office.SparklineItem{{Cell: "D2", Range: "Inputs!A1:C1"}}}); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "authored.xlsx")
	if err := pkg.SaveNew(src); err != nil {
		t.Fatal(err)
	}
	before := contents(t, src)
	if _, err := office.OfficePreviewCopy(src, filepath.Join(t.TempDir(), "stale.xlsx")); err == nil || !strings.Contains(err.Error(), "Recalculate sparkline source") {
		t.Fatal(err)
	}
	var calls []string
	ctx := withDeps(dependencies{refresh: func(ctx context.Context, in, dst string, whole bool) error {
		suffix := filepath.Ext(dst)
		if whole != (suffix == ".pdf") {
			t.Fatal("whole sheets", whole, suffix)
		}
		calls = append(calls, suffix)
		p, err := opc.Open(in)
		if err != nil {
			return err
		}
		if suffix == ".xlsx" {
			d, e := p.XML("xl/worksheets/sheet1.xml")
			if e != nil {
				return e
			}
			found := false
			for _, c := range office.All(d.Root(), office.NSS, "c") {
				if c.SelectAttrValue("r", "") == "B1" {
					v := office.Child(c, office.NSS, "v")
					if v == nil {
						t.Fatal("original callback requires existing B1 cache element")
					}
					v.SetText("6")
					found = true
					break // lxml find selects the first matching cell
				}
			}
			if !found {
				t.Fatal("original callback requires B1")
			}
			if e = p.SetXML("xl/worksheets/sheet1.xml", d); e != nil {
				return e
			}
			return p.SaveNew(dst)
		}
		d, err := p.XML("xl/drawings/preview1.xml")
		if err != nil {
			return err
		}
		pts := office.All(d.Root(), office.NSA, "pt")
		if len(pts) < 3 {
			t.Fatal("missing sparkline points")
		}
		y := func(i int) int {
			v, e := strconv.Atoi(pts[i].SelectAttrValue("y", ""))
			if e != nil {
				t.Fatal(e)
			}
			return v
		}
		if y(1) >= y(0) {
			t.Fatal("middle point must be above first")
		}
		if y(1) >= y(2) {
			t.Fatal("middle point must be above last")
		}
		return os.WriteFile(dst, referenceCallbackFixture(t, "stale-1.pdf"), 0600)
	}})
	pages, err := RenderFile(ctx, src, filepath.Join(t.TempDir(), "rendered"), "", "", pdf.RenderOptions{MaxPixels: 1600})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{".xlsx", ".pdf"}) {
		t.Fatal(calls)
	}
	if len(pages) != 1 {
		t.Fatal(pages)
	}
	assertUnchanged(t, src, before)
}

func TestReferenceExternalDataNeverRefreshed(t *testing.T) {
	src := referencePackage(t, "linked.xlsx", map[string]string{"xl/workbook.xml": "<workbook/>", "xl/_rels/workbook.xml.rels": `<Relationships><Relationship TargetMode="External" Type="externalLink" Target="https://example.invalid/private"/></Relationships>`})
	out := filepath.Join(t.TempDir(), "result")
	ctx := withDeps(dependencies{refresh: func(context.Context, string, string, bool) error {
		t.Fatal("external workbook reached refresh engine")
		return nil
	}})
	if _, e := Convert(ctx, src, out, Options{Recalculate: true}); e == nil || !strings.Contains(e.Error(), "External resources") {
		t.Fatal(e)
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("created output", e)
	}
}

func TestReferenceMalformedLiteralChartRejectsDuplicateIndexes(t *testing.T) {
	src := testsupport.LiteralChartFixture(t)
	err := testsupport.RewriteStoredZIP(src, func(parts *testsupport.OrderedZIP) error {
		for _, part := range parts.Parts {
			if part.Name != "ppt/charts/chart1.xml" {
				continue
			}
			chart := []byte(part.Data)
			corrupt := bytes.ReplaceAll(chart, []byte(`idx="1"`), []byte(`idx="0"`))
			if bytes.Equal(chart, corrupt) {
				return fmt.Errorf("fixture did not contain second point")
			}
			parts.Set(part.Name, corrupt)
			return nil
		}
		return fmt.Errorf("missing chart")
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	report, err := office.PackageReport(src)
	if err != nil {
		t.Fatal(err)
	}
	target := "powerpoint"
	if _, err := office.VerifyPresentation(p, report, office.SlideRequirements{TargetApplication: &target}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("verify: %v", err)
	}
	out := filepath.Join(t.TempDir(), "bad.pptx")
	if _, err := MaterializeCharts(src, out); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("materialize: %v", err)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Error("bad output remains", err)
	}
}

// The original materialization method retries the public operation, not the
// package writer. Preserve both files and expose FileExistsError's Go analogue.
func TestReferenceMaterializeChartsOverwriteBoundary(t *testing.T) {
	source := testsupport.LiteralChartFixture(t)
	verify := func(filename string, target *string) (*office.SlideCheck, error) {
		t.Helper()
		pkg, err := opc.Open(filename)
		if err != nil {
			t.Fatal(err)
		}
		report, err := office.PackageReportOf(pkg)
		if err != nil {
			t.Fatal(err)
		}
		return office.VerifyPresentation(pkg, report, office.SlideRequirements{TargetApplication: target})
	}
	if _, err := verify(source, nil); err == nil || !strings.Contains(err.Error(), "materialization") {
		t.Fatalf("literal chart must require materialization: %v", err)
	}
	target := "powerpoint"
	initial, err := verify(source, &target)
	if err != nil || len(initial.ChartData) == 0 {
		t.Fatalf("explicit target: %v, %v", initial, err)
	}
	if initial.ChartData[0].(map[string]any)["workbook_verified"] != false {
		t.Fatal("literal chart unexpectedly workbook-verified")
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "materialized.pptx")
	if _, err := MaterializeCharts(source, destination); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	result, err := verify(destination, nil)
	if err != nil || len(result.ChartData) == 0 {
		t.Fatalf("materialized artifact verification: %v, %v", result, err)
	}
	chart := result.ChartData[0].(map[string]any)
	if chart["references"].(int) <= 0 || chart["literal_series"] != 0 {
		t.Fatalf("materialized chart: %v", chart)
	}
	preserved, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, preserved) {
		t.Fatalf("source changed before overwrite attempt: %v", err)
	}
	if _, err := MaterializeCharts(source, destination); !errors.Is(err, os.ErrExist) {
		t.Fatalf("want file-exists error, got %v", err)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("source changed: %v", err)
	}
	after, err = os.ReadFile(destination)
	if err != nil || !bytes.Equal(output, after) {
		t.Fatalf("destination changed: %v", err)
	}
}

// Original alternate-chart lineage assertions, exercised through the public
// saved-file operation rather than only its in-memory office implementation.
func TestReferenceAlternateChartPublicMaterializationLineage(t *testing.T) {
	source := "../../testdata/fixtures/reference-bar.pptx"
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := testsupport.PresentationOutput(t, t.TempDir())
	if _, err := MaterializeCharts(source, destination); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	charts, err := office.VerifyChartData(pkg, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(charts) != 1 {
		t.Fatalf("charts=%d; want 1", len(charts))
	}
	if charts[0].(map[string]any)["references"].(int) <= 0 {
		t.Fatalf("no references: %v", charts[0])
	}
	report, err := office.PackageReportOf(pkg)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(report["external_relationships"])
	if err != nil || string(raw) != "[]" {
		t.Fatalf("external relationships: %s (%v)", raw, err)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("source changed: %v", err)
	}
}
