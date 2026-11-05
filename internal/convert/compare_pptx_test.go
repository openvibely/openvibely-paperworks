package convert

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

type comparePPTXShape struct {
	id   string
	name string
	runs []string
	bold bool
}

type comparePPTXSlide struct {
	id     string
	shapes []comparePPTXShape
}

func writeComparePPTX(t *testing.T, slides []comparePPTXSlide) string {
	t.Helper()
	var ids, rels strings.Builder
	parts := map[string]string{
		"_rels/.rels": `<Relationships xmlns="` + opc.NSRel + `"><Relationship Id="office" Type="` + officeDocumentRelationship + `" Target="ppt/presentation.xml"/></Relationships>`,
	}
	for i, slide := range slides {
		relID := "rId" + itoa(i+1)
		ids.WriteString(`<p:sldId id="` + slide.id + `" r:id="` + relID + `"/>`)
		rels.WriteString(`<Relationship Id="` + relID + `" Type="` + slideRelationship + `" Target="slides/slide` + itoa(i+1) + `.xml"/>`)
		var shapes strings.Builder
		for _, shape := range slide.shapes {
			shapeID := ""
			if shape.id != "" {
				shapeID = ` id="` + shape.id + `"`
			}
			shapeName := ""
			if shape.name != "" {
				shapeName = ` name="` + shape.name + `"`
			}
			shapes.WriteString(`<p:sp><p:nvSpPr><p:cNvPr` + shapeID + shapeName + `/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p>`)
			for _, run := range shape.runs {
				rPr := ""
				if shape.bold {
					rPr = `<a:rPr b="1"/>`
				}
				shapes.WriteString(`<a:r>` + rPr + `<a:t>` + run + `</a:t></a:r>`)
			}
			shapes.WriteString(`</a:p></p:txBody></p:sp>`)
		}
		parts["ppt/slides/slide"+itoa(i+1)+".xml"] = `<p:sld xmlns:p="` + office.NSP + `" xmlns:a="` + office.NSA + `"><p:cSld><p:spTree><p:nvGrpSpPr/><p:grpSpPr/>` + shapes.String() + `</p:spTree></p:cSld></p:sld>`
	}
	parts["ppt/presentation.xml"] = `<p:presentation xmlns:p="` + office.NSP + `" xmlns:r="` + office.NSR + `"><p:sldIdLst>` + ids.String() + `</p:sldIdLst></p:presentation>`
	parts["ppt/_rels/presentation.xml.rels"] = `<Relationships xmlns="` + opc.NSRel + `">` + rels.String() + `</Relationships>`
	return referencePackage(t, "compare.pptx", parts)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out [20]byte
	i := len(out)
	for n > 0 {
		i--
		out[i] = byte('0' + n%10)
		n /= 10
	}
	return string(out[i:])
}

func comparePPTXText(t *testing.T, before, after []comparePPTXSlide) ([]PPTXTextChange, string) {
	t.Helper()
	beforePath := writeComparePPTX(t, before)
	afterPath := writeComparePPTX(t, after)
	beforeSlides, err := pptxTextSlides(beforePath)
	if err != nil {
		t.Fatal(err)
	}
	afterSlides, err := pptxTextSlides(afterPath)
	if err != nil {
		t.Fatal(err)
	}
	changes := pptxChanges(beforeSlides, afterSlides)
	return changes, pptxTextDiff(beforePath, afterPath, changes)
}

func TestComparePPTXTextEditsAndShapeIdentity(t *testing.T) {
	before := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{
		{id: "2", name: "Title", runs: []string{"Draft"}},
		{id: "3", name: "Total", runs: []string{"Total ", "100"}},
	}}}
	after := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{
		{id: "2", name: "Title", runs: []string{"Final"}},
		{id: "3", name: "Total", runs: []string{"Total ", "250"}},
	}}}
	changes, diff := comparePPTXText(t, before, after)
	if len(changes) != 2 {
		t.Fatalf("got %d text changes: %#v", len(changes), changes)
	}
	want := map[string][2]string{"2": {"Draft", "Final"}, "3": {"Total 100", "Total 250"}}
	for _, change := range changes {
		if change.Status != "changed" || change.Slide != 1 || change.ShapeID == "" {
			t.Errorf("missing shape/slide change identity: %#v", change)
		}
		if got, ok := want[change.ShapeID]; !ok || change.Before != got[0] || change.After != got[1] {
			t.Errorf("unexpected before/after text: %#v", change)
		} else {
			delete(want, change.ShapeID)
		}
	}
	if len(want) != 0 || !strings.Contains(diff, "-Total 100") || !strings.Contains(diff, "+Total 250") {
		t.Fatalf("missing readable word/numeric edits: remaining=%v diff=%q", want, diff)
	}
}

func TestComparePPTXRunBoundariesAreNotTextChanges(t *testing.T) {
	before := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", runs: []string{"Split ", "across runs"}}}}}
	after := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", runs: []string{"Split across ", "runs"}}}}}
	changes, _ := comparePPTXText(t, before, after)
	if len(changes) != 0 {
		t.Fatalf("run-boundary-only edit reported as text change: %#v", changes)
	}
}

func TestComparePPTXAddedAndRemovedShapes(t *testing.T) {
	before := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{
		{id: "2", name: "Keep", runs: []string{"keep"}},
		{id: "3", name: "Old", runs: []string{"removed text"}},
	}}}
	after := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{
		{id: "2", name: "Keep", runs: []string{"keep"}},
		{id: "4", name: "New", runs: []string{"added text"}},
	}}}
	changes, diff := comparePPTXText(t, before, after)
	if len(changes) != 2 {
		t.Fatalf("got %d changes: %#v", len(changes), changes)
	}
	statuses := map[string]bool{}
	for _, change := range changes {
		statuses[change.Status] = true
		switch change.Status {
		case "removed":
			if change.BeforeShapeID != "3" || change.AfterShapeID != "" || change.Before != "removed text" {
				t.Errorf("bad removed shape record: %#v", change)
			}
		case "added":
			if change.AfterShapeID != "4" || change.BeforeShapeID != "" || change.After != "added text" {
				t.Errorf("bad added shape record: %#v", change)
			}
		default:
			t.Errorf("expected add/remove status, got %#v", change)
		}
	}
	if !statuses["added"] || !statuses["removed"] || !strings.Contains(diff, "removed shape id=3") || !strings.Contains(diff, "added shape id=4") {
		t.Fatalf("added/removed shapes not distinguished: %#v diff=%q", changes, diff)
	}
}

func TestComparePPTXDifferentShapeIDsAreReportedAsReplacement(t *testing.T) {
	cases := []struct {
		name   string
		before comparePPTXShape
		after  comparePPTXShape
	}{
		{
			name:   "same text with different IDs",
			before: comparePPTXShape{id: "3", name: "Old", runs: []string{"same text"}},
			after:  comparePPTXShape{id: "4", name: "New", runs: []string{"same text"}},
		},
		{
			name:   "same name with different IDs and changed text",
			before: comparePPTXShape{id: "3", name: "Shared", runs: []string{"before"}},
			after:  comparePPTXShape{id: "4", name: "Shared", runs: []string{"after"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{tc.before}}}
			after := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{tc.after}}}
			changes, diff := comparePPTXText(t, before, after)
			if len(changes) != 2 {
				t.Fatalf("replacement should produce removal and addition: %#v", changes)
			}
			removed, added := false, false
			for _, change := range changes {
				switch change.Status {
				case "removed":
					removed = change.BeforeShapeID == tc.before.id && change.Before == strings.Join(tc.before.runs, "") && change.AfterShapeID == ""
				case "added":
					added = change.AfterShapeID == tc.after.id && change.After == strings.Join(tc.after.runs, "") && change.BeforeShapeID == ""
				case "changed":
					t.Errorf("different-ID shapes were paired as a text edit: %#v", change)
				}
			}
			if !removed || !added || !strings.Contains(diff, "removed shape id="+tc.before.id) || !strings.Contains(diff, "added shape id="+tc.after.id) {
				t.Fatalf("replacement was not reported as removal plus addition: %#v diff=%q", changes, diff)
			}
		})
	}
}

func TestComparePPTXIdenticalAndStyleOnlyChanges(t *testing.T) {
	base := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", name: "Title", runs: []string{"same text"}}}}}
	for _, tc := range []struct {
		name  string
		after []comparePPTXSlide
	}{
		{name: "identical", after: base},
		{name: "formatting-only", after: []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", name: "Title", runs: []string{"same text"}, bold: true}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changes, diff := comparePPTXText(t, base, tc.after)
			if len(changes) != 0 || diff != "" {
				t.Fatalf("text-only comparison reported %s as text change: %#v diff=%q", tc.name, changes, diff)
			}
		})
	}
}

func TestComparePPTXSlideInsertionAndReordering(t *testing.T) {
	before := []comparePPTXSlide{
		{id: "256", shapes: []comparePPTXShape{{id: "2", name: "A", runs: []string{"Alpha"}}}},
		{id: "257", shapes: []comparePPTXShape{{id: "2", name: "B", runs: []string{"Beta"}}}},
		{id: "258", shapes: []comparePPTXShape{{id: "2", name: "C", runs: []string{"Gamma"}}}},
	}
	after := []comparePPTXSlide{
		{id: "257", shapes: []comparePPTXShape{{id: "2", name: "B", runs: []string{"Beta updated"}}}},
		{id: "259", shapes: []comparePPTXShape{{id: "9", name: "New", runs: []string{"Inserted"}}}},
		{id: "256", shapes: []comparePPTXShape{{id: "2", name: "A", runs: []string{"Alpha"}}}},
		{id: "258", shapes: []comparePPTXShape{{id: "2", name: "C", runs: []string{"Gamma"}}}},
	}
	changes, _ := comparePPTXText(t, before, after)
	if len(changes) != 2 {
		t.Fatalf("got %d changes: %#v", len(changes), changes)
	}
	changed, added := false, false
	for _, change := range changes {
		if change.Status == "changed" && change.BeforeSlide == 2 && change.AfterSlide == 1 && change.ShapeID == "2" && change.Before == "Beta" && change.After == "Beta updated" {
			changed = true
		}
		if change.Status == "added" && change.AfterSlide == 2 && change.AfterShapeID == "9" && change.After == "Inserted" {
			added = true
		}
	}
	if !changed || !added {
		t.Fatalf("slide insertion/reordering misattributed text: %#v", changes)
	}
}

func TestComparePPTXSameGapReplacementReportsSlideRemovalAndAddition(t *testing.T) {
	before := []comparePPTXSlide{
		{id: "256", shapes: []comparePPTXShape{{id: "2", name: "A", runs: []string{"Alpha"}}}},
		{id: "257", shapes: []comparePPTXShape{{id: "2", name: "B", runs: []string{"Beta"}}}},
	}
	after := []comparePPTXSlide{
		{id: "256", shapes: []comparePPTXShape{{id: "2", name: "A", runs: []string{"Alpha"}}}},
		{id: "258", shapes: []comparePPTXShape{{id: "2", name: "Replacement", runs: []string{"Xray"}}}},
	}
	changes, diff := comparePPTXText(t, before, after)
	if len(changes) != 2 {
		t.Fatalf("got %d changes: %#v", len(changes), changes)
	}
	removed, added := false, false
	for _, change := range changes {
		switch change.Status {
		case "removed":
			removed = change.BeforeSlide == 2 && change.BeforeShapeID == "2" && change.Before == "Beta" && change.After == ""
		case "added":
			added = change.AfterSlide == 2 && change.AfterShapeID == "2" && change.After == "Xray" && change.Before == ""
		case "changed":
			t.Errorf("different-ID slides were paired as a text edit: %#v", change)
		}
	}
	if !removed || !added || !strings.Contains(diff, "slide 2 removed shape id=2") || !strings.Contains(diff, "slide 2 added shape id=2") {
		t.Fatalf("same-gap replacement was not reported as removal plus addition: %#v diff=%q", changes, diff)
	}
}

func TestComparePPTXReorderedSlidesPreferTextIdentityOverSlideIDs(t *testing.T) {
	before := []comparePPTXSlide{
		{id: "256", shapes: []comparePPTXShape{{id: "2", runs: []string{"Alpha"}}}},
		{id: "257", shapes: []comparePPTXShape{{id: "2", runs: []string{"Beta"}}}},
	}
	after := []comparePPTXSlide{
		{id: "256", shapes: []comparePPTXShape{{id: "2", runs: []string{"Beta"}}}},
		{id: "257", shapes: []comparePPTXShape{{id: "2", runs: []string{"Alpha"}}}},
	}
	changes, _ := comparePPTXText(t, before, after)
	if len(changes) != 0 {
		t.Fatalf("reordered text-identical slides were misattributed: %#v", changes)
	}
}

func TestComparePPTXTextClearedOrAddedWithinExistingShape(t *testing.T) {
	cases := []struct {
		name   string
		before []string
		after  []string
		old    string
		new    string
	}{
		{name: "cleared", before: []string{"Existing text"}, old: "Existing text"},
		{name: "added", after: []string{"New text"}, new: "New text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", name: "Text box", runs: tc.before}}}}
			after := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", name: "Text box", runs: tc.after}}}}
			changes, diff := comparePPTXText(t, before, after)
			if len(changes) != 1 {
				t.Fatalf("got %d changes: %#v", len(changes), changes)
			}
			change := changes[0]
			if change.Status != "changed" || change.ShapeID != "2" || change.Before != tc.old || change.After != tc.new {
				t.Fatalf("text transition lost shape identity or empty side: %#v", change)
			}
			encoded, err := json.Marshal(change)
			if err != nil {
				t.Fatal(err)
			}
			var report map[string]any
			if err := json.Unmarshal(encoded, &report); err != nil {
				t.Fatal(err)
			}
			if report["before"] != tc.old || report["after"] != tc.new {
				t.Fatalf("JSON report omitted an empty before/after value: %s", encoded)
			}
			if !strings.Contains(diff, "-"+tc.old+"\n") || !strings.Contains(diff, "+"+tc.new+"\n") {
				t.Fatalf("text diff omitted an empty-side marker: %q", diff)
			}
		})
	}
}

func TestComparePPTXShapeFallbackWhenIDsAreMissing(t *testing.T) {
	before := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{name: "Stable name", runs: []string{"before"}}}}}
	after := []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{name: "Stable name", runs: []string{"after"}}}}}
	changes, _ := comparePPTXText(t, before, after)
	if len(changes) != 1 || changes[0].Status != "changed" || changes[0].BeforeShapeID != "" || changes[0].AfterShapeID != "" || changes[0].Before != "before" || changes[0].After != "after" {
		t.Fatalf("shape name fallback failed when IDs were missing: %#v", changes)
	}

	before = []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{runs: []string{"before"}}}}}
	after = []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{runs: []string{"after"}}}}}
	changes, _ = comparePPTXText(t, before, after)
	if len(changes) != 1 || changes[0].Status != "changed" || changes[0].Before != "before" || changes[0].After != "after" {
		t.Fatalf("positional shape fallback failed when IDs and names were missing: %#v", changes)
	}
}

func writeComparePNG(t *testing.T, path string, mark bool) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 12, 12))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	if mark {
		img.Set(5, 5, color.Black)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestComparePPTXWritesTextDiffAndPreservesVisualDiff(t *testing.T) {
	dir := t.TempDir()
	before := writeComparePPTX(t, []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", name: "Title", runs: []string{"Before"}}}}})
	after := writeComparePPTX(t, []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", name: "Title", runs: []string{"After"}}}}})
	beforePNG, afterPNG := filepath.Join(dir, "before.png"), filepath.Join(dir, "after.png")
	writeComparePNG(t, beforePNG, false)
	writeComparePNG(t, afterPNG, true)
	ctx := withDeps(dependencies{
		convert: func(_ context.Context, _, out string, _ Options) (map[string]any, error) {
			return map[string]any{"output": filepath.Join(out, "mock.pdf")}, nil
		},
		render: func(_ context.Context, pdfPath, _ string, _ pdf.RenderOptions) ([]string, error) {
			if strings.Contains(pdfPath, filepath.Join("before", "mock.pdf")) {
				return []string{beforePNG}, nil
			}
			return []string{afterPNG}, nil
		},
	})
	result, err := Compare(ctx, before, after, filepath.Join(dir, "report"), pdf.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result["text_changed"] != true || result["slide_matching"] != pptxSlideMatchingDescription {
		t.Fatalf("missing PPTX text comparison fields: %#v", result)
	}
	pages := result["pages"].([]PageChange)
	if len(pages) != 1 || pages[0].Status != "changed" || pages[0].Image == "" {
		t.Fatalf("visual page diff was not retained: %#v", pages)
	}
	diff, err := os.ReadFile(result["text_diff"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(diff), "slide 1 changed shape id=2") || !strings.Contains(string(diff), "-Before") || !strings.Contains(string(diff), "+After") {
		t.Fatalf("PPTX text diff is incomplete: %q", diff)
	}
	if _, err := os.Stat(filepath.Join(dir, "report", "text-before.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "report", "text-after.txt")); err != nil {
		t.Fatal(err)
	}

	styleBefore := writeComparePPTX(t, []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", name: "Title", runs: []string{"Same text"}}}}})
	styleAfter := writeComparePPTX(t, []comparePPTXSlide{{id: "256", shapes: []comparePPTXShape{{id: "2", name: "Title", runs: []string{"Same text"}, bold: true}}}})
	styleResult, err := Compare(ctx, styleBefore, styleAfter, filepath.Join(dir, "style-report"), pdf.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if styleResult["text_changed"] != false {
		t.Fatalf("formatting-only edit reported as text change: %#v", styleResult["text_changes"])
	}
	stylePages := styleResult["pages"].([]PageChange)
	if len(stylePages) != 1 || stylePages[0].Status != "changed" {
		t.Fatalf("formatting-only visual diff was lost: %#v", stylePages)
	}
	styleDiff, err := os.ReadFile(styleResult["text_diff"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if len(styleDiff) != 0 {
		t.Fatalf("formatting-only edit produced text diff: %q", styleDiff)
	}
}
