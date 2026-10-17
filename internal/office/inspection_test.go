package office

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func referenceInspection(t *testing.T, parts []referenceCheckPart) map[string]any {
	t.Helper()
	return referenceInspectionNamed(t, "source.zip", parts)
}

func referenceInspectionNamed(t *testing.T, name string, parts []referenceCheckPart) map[string]any {
	t.Helper()
	report, unchanged := referenceInspectionRead(t, name, parts)
	unchanged()
	return report
}

func referenceInspectionRead(t *testing.T, name string, parts []referenceCheckPart) (map[string]any, func()) {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, part := range parts {
		w, err := z.CreateHeader(&zip.FileHeader{Name: part.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(part.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(source, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := PackageReport(source)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out, func() {
		t.Helper()
		after, err := os.ReadFile(source)
		if err != nil || !bytes.Equal(b.Bytes(), after) {
			t.Fatal("source modified", err)
		}
	}
}

// Mirrors DocumentChecks.test_workbook_errors_and_missing_caches and
// test_empty_string_formula_result_is_a_valid_cache with the original minimal packages.
func TestReferenceWorkbookInspection(t *testing.T) {
	workbook := `<workbook xmlns="` + NSS + `"><sheets><sheet name="Inputs"/></sheets></workbook>`
	for _, tc := range []struct {
		name, cells string
		empty       bool
	}{
		{"errors-and-caches", `<c r="A1"><f>1+1</f><v>2</v></c><c r="A2"><f>A1*2</f></c><c r="A3" t="e"><f>1/0</f><v>#DIV/0!</v></c>`, false},
		{"empty-string", `<c r="A1" t="str"><f>IF(1=1,"",1)</f><v/></c>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			book := workbook
			if tc.empty {
				book = `<workbook xmlns="` + NSS + `"/>`
			}
			filename := "book.xlsx"
			if tc.empty {
				filename = "empty-result.xlsx"
			}
			r := referenceInspectionNamed(t, filename, []referenceCheckPart{{"xl/workbook.xml", book}, {"xl/worksheets/sheet1.xml", `<worksheet xmlns="` + NSS + `"><sheetData><row>` + tc.cells + `</row></sheetData></worksheet>`}})
			issues := r["issues"].([]any)
			if tc.empty {
				if len(issues) != 0 {
					t.Fatal(issues)
				}
				return
			}
			if !reflect.DeepEqual(r["sheets"], []any{"Inputs"}) {
				t.Fatal(r["sheets"])
			}
			if len(r["formulas"].([]any)) != 3 {
				t.Fatal(r)
			}
			errors := r["errors"].([]any)
			if len(errors) == 0 || errors[0].(map[string]any)["error"] != "#DIV/0!" {
				t.Fatal(errors)
			}
			if len(issues) != 1 {
				t.Fatal(issues)
			}
			if !strings.Contains(issues[0].(string), "A2") {
				t.Fatal(issues[0])
			}
		})
	}
}

// Mirrors DocumentChecks.test_word_revisions_and_accessibility.
func TestReferenceWordInspection(t *testing.T) {
	r := referenceInspectionNamed(t, "review.docx", []referenceCheckPart{
		{"word/document.xml", `<w:document xmlns:w="` + NSW + `" xmlns:wp="urn:drawing"><w:body><w:ins><w:r><w:t>New</w:t></w:r></w:ins><w:del><w:r><w:delText>Old</w:delText></w:r></w:del><wp:docPr name="Chart"/><w:tbl><w:tr/></w:tbl></w:body></w:document>`},
		{"word/comments.xml", `<w:comments xmlns:w="` + NSW + `"><w:comment/></w:comments>`},
	})
	for _, key := range []string{"tracked_insertions", "tracked_deletions", "comments"} {
		if r[key] != float64(1) {
			t.Fatal(key, r[key])
		}
	}
	if len(r["issues"].([]any)) != 2 {
		t.Fatal(r["issues"])
	}
}

// Mirrors DocumentChecks.test_slide_bounds, test_group_does_not_hide_out_of_bounds_sibling,
// and all eight subcases of test_nested_scaled_rotated_groups_and_graphic_frames.
func TestReferenceSlideInspection(t *testing.T) {
	shape := func(x, y, w, h, rot int, frame bool) string {
		tag := "a:xfrm"
		if frame {
			tag = "p:xfrm"
		}
		return fmt.Sprintf(`<p:sp><p:spPr><%s rot="%d"><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></%s></p:spPr></p:sp>`, tag, rot, x, y, w, h, tag)
	}
	group := func(content string, x, y, w, h, rot, flip int) string {
		return fmt.Sprintf(`<p:grpSp><p:grpSpPr><a:xfrm rot="%d" flipH="%d"><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/><a:chOff x="100" y="100"/><a:chExt cx="200" cy="200"/></a:xfrm></p:grpSpPr>%s</p:grpSp>`, rot, flip, x, y, w, h, content)
	}
	standard := func(content string) string { return group(content, 10, 10, 40, 40, 0, 0) }
	outside := `<a:xfrm><a:off x="90" y="10"/><a:ext cx="20" cy="20"/></a:xfrm>`
	for _, tc := range []struct {
		name, objects string
		count         int
	}{
		{"slide-bounds", outside, 1},
		{"sibling", `<p:grpSp><p:sp><p:spPr>` + outside + `</p:spPr></p:sp></p:grpSp><p:sp><p:spPr>` + outside + `</p:spPr></p:sp>`, 1},
		{"local coordinates inside", standard(shape(100, 100, 200, 200, 0, false)), 0},
		{"scaled child outside", standard(shape(600, 100, 200, 200, 0, false)), 1},
		{"nested inside", standard(group(shape(100, 100, 200, 200, 0, false), 100, 100, 200, 200, 0, 0)), 0},
		{"nested outside", standard(group(shape(600, 100, 200, 200, 0, false), 100, 100, 200, 200, 0, 0)), 1},
		{"rotated rectangle outside", shape(0, 0, 100, 20, 45*60000, false), 1},
		{"rotated group outside", group(shape(100, 100, 200, 200, 0, false), 0, 0, 80, 80, 45*60000, 0), 1},
		{"flipped group inside", group(shape(100, 100, 100, 100, 0, false), 10, 10, 40, 40, 0, 1), 0},
		{"graphic frame outside", shape(95, 5, 20, 20, 0, true), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filename := "geometry.pptx"
			presentation := `<p:presentation xmlns:p="` + NSP + `"><p:sldSz cx="100" cy="100"/></p:presentation>`
			objects := `<p:cSld><p:spTree>` + tc.objects + `</p:spTree></p:cSld>`
			if tc.name == "slide-bounds" {
				filename = "slides.pptx"
				presentation = `<p:presentation xmlns:p="` + NSP + `"><p:sldIdLst><p:sldId/></p:sldIdLst><p:sldSz cx="100" cy="100"/></p:presentation>`
				objects = tc.objects
			} else if tc.name == "sibling" {
				filename = "grouped.pptx"
			}
			r, unchanged := referenceInspectionRead(t, filename, []referenceCheckPart{
				{"ppt/presentation.xml", presentation},
				{"ppt/slides/slide1.xml", `<p:sld xmlns:p="` + NSP + `" xmlns:a="` + NSA + `">` + objects + `</p:sld>`},
			})
			if tc.name == "slide-bounds" && r["slides"] != float64(1) {
				t.Fatal(r["slides"])
			}
			issues := r["issues"].([]any)
			if tc.name == "sibling" {
				if !reflect.DeepEqual(issues, []any{"ppt/slides/slide1.xml: object exceeds slide bounds"}) {
					t.Fatal(issues)
				}
			} else if len(issues) != tc.count {
				t.Fatal(issues)
			}
			// Original nested-geometry source preservation follows its issue assertion.
			unchanged()
		})
	}
}
