package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/pptx"
)

func makeSlideSelectionDeck(t *testing.T) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "selection.pptx")
	slides := make([]pptx.Slide, 4)
	for i := range slides {
		slides[i].Elements = []pptx.Element{{Type: "text", Text: fmt.Sprintf("Slide %d", i+1), X: 1, Y: 1, W: 5, H: 1, FontSize: 24}}
	}
	slides[1].Elements = []pptx.Element{{Type: "chart", X: 1, Y: 2, W: 6, H: 4, Chart: &pptx.Chart{Type: "bar", Categories: []string{"A", "B"}, Series: []pptx.Series{{Name: "Sales", Values: []float64{10, 20}}}}}}
	slides[2].Elements = []pptx.Element{{Type: "table", X: 1, Y: 2, W: 6, H: 3, Rows: [][]string{{"Name", "Value"}, {"A", "10"}}}}
	if err := pptx.Create(context.Background(), pptx.Spec{Slides: slides}, dir, file); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	parts, err := office.PresentationSlides(pkg)
	if err != nil {
		t.Fatal(err)
	}
	addInheritedPreviewFixture(t, pkg, parts[3])
	decorated := filepath.Join(dir, "decorated.pptx")
	if err := pkg.SaveNew(decorated); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(decorated, file); err != nil {
		t.Fatal(err)
	}
	return file, parts
}

func addInheritedPreviewFixture(t *testing.T, pkg *opc.Package, slidePart string) {
	t.Helper()
	rels, err := pkg.Rels(slidePart)
	if err != nil {
		t.Fatal(err)
	}
	layoutPart := ""
	for _, rel := range rels {
		if strings.HasSuffix(rel.Type, "/slideLayout") {
			layoutPart = opc.Resolve(slidePart, rel.Target)
			break
		}
	}
	if layoutPart == "" {
		t.Fatal("slide layout relationship missing")
	}
	layoutRels, err := pkg.Rels(layoutPart)
	if err != nil {
		t.Fatal(err)
	}
	masterPart := ""
	for _, rel := range layoutRels {
		if strings.HasSuffix(rel.Type, "/slideMaster") {
			masterPart = opc.Resolve(layoutPart, rel.Target)
			break
		}
	}
	if masterPart == "" {
		t.Fatal("slide master relationship missing")
	}
	slide, err := pkg.XML(slidePart)
	if err != nil {
		t.Fatal(err)
	}
	shapes := office.All(slide.Root(), office.NSP, "sp")
	if len(shapes) == 0 {
		t.Fatal("slide text shape missing")
	}
	shape := shapes[len(shapes)-1]
	nv := office.Child(office.Child(shape, office.NSP, "nvSpPr"), office.NSP, "nvPr")
	placeholder := nv.CreateElement("p:ph")
	placeholder.CreateAttr("type", "body")
	placeholder.CreateAttr("idx", "1")
	layoutShape := shape.Copy()
	layoutShape.CreateAttr("xmlns:p", office.NSP)
	layoutShape.CreateAttr("xmlns:a", office.NSA)
	masterShape := shape.Copy()
	masterShape.CreateAttr("xmlns:p", office.NSP)
	masterShape.CreateAttr("xmlns:a", office.NSA)
	layout, err := pkg.XML(layoutPart)
	if err != nil {
		t.Fatal(err)
	}
	master, err := pkg.XML(masterPart)
	if err != nil {
		t.Fatal(err)
	}
	layoutTree := office.Child(office.Child(layout.Root(), office.NSP, "cSld"), office.NSP, "spTree")
	masterTree := office.Child(office.Child(master.Root(), office.NSP, "cSld"), office.NSP, "spTree")
	layoutTree.AddChild(layoutShape)
	masterTree.AddChild(masterShape)
	if err := pkg.SetXML(slidePart, slide); err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetXML(layoutPart, layout); err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetXML(masterPart, master); err != nil {
		t.Fatal(err)
	}
}

func TestSlidePreviewSelectionLimitsLayoutAndRenderWork(t *testing.T) {
	file, originalParts := makeSlideSelectionDeck(t)
	tests := []struct {
		name      string
		selection string
		want      []int
	}{
		{name: "default all slides", want: []int{1, 2, 3, 4}},
		{name: "one chart slide", selection: "2", want: []int{2}},
		{name: "discontiguous chart table inherited", selection: "2,3,4", want: []int{2, 3, 4}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "preview")
			wantParts := make([]string, 0, len(tc.want))
			for _, number := range tc.want {
				wantParts = append(wantParts, originalParts[number-1])
			}
			rendered := 0
			render := previewFileRenderer(func(_ context.Context, source, renderDir, _, _ string, _ pdf.RenderOptions) ([]string, error) {
				rendered++
				snapshot, err := opc.Open(source)
				if err != nil {
					return nil, err
				}
				parts, err := office.PresentationSlides(snapshot)
				if err != nil {
					return nil, err
				}
				if !reflect.DeepEqual(parts, wantParts) {
					return nil, fmt.Errorf("renderer saw slide parts %v, want %v", parts, wantParts)
				}
				if err := convert.VerifyOfficeChartPreview(context.Background(), source, ""); err != nil {
					return nil, fmt.Errorf("selected chart relationships: %w", err)
				}
				if err := os.MkdirAll(renderDir, 0755); err != nil {
					return nil, err
				}
				pages := make([]string, 0, len(parts))
				for i := range parts {
					page := filepath.Join(renderDir, fmt.Sprintf("page-%d.png", i+1))
					if err := writePreview(page, image.NewRGBA(image.Rect(0, 0, 16, 9)), "png"); err != nil {
						return nil, err
					}
					pages = append(pages, page)
				}
				return pages, nil
			})
			args := Args{Pos: []string{file}, Flags: map[string]string{"output": out}}
			if tc.selection != "" {
				args.Flags["slides"] = tc.selection
			}
			result, err := runSlidePreview(context.Background(), args, render)
			if err != nil {
				t.Fatal(err)
			}
			if rendered != 1 {
				t.Fatalf("renderer calls %d, want one", rendered)
			}
			pairs := result.(map[string]any)["slides"].([]map[string]string)
			if len(pairs) != len(tc.want) {
				t.Fatalf("rendered %d slides, want %d", len(pairs), len(tc.want))
			}
			for i, number := range tc.want {
				stem := fmt.Sprintf("slide-%d", number)
				if filepath.Base(pairs[i]["image"]) != stem+".png" || filepath.Base(pairs[i]["layout"]) != stem+".layout.json" {
					t.Fatalf("slide %d lost original numbering: %#v", number, pairs[i])
				}
				data, err := os.ReadFile(pairs[i]["layout"])
				if err != nil {
					t.Fatal(err)
				}
				var report map[string]any
				if err := json.Unmarshal(data, &report); err != nil {
					t.Fatal(err)
				}
				if report["slide"] != float64(number) {
					t.Fatalf("layout slide number %v, want %d", report["slide"], number)
				}
				elements := report["elements"].([]any)
				if number == 2 && !hasElementKind(elements, "graphicFrame") {
					t.Fatal("selected chart slide layout lacks its graphic frame")
				}
				if number == 3 && !hasTableContent(elements) {
					t.Fatal("selected table slide layout lacks table content")
				}
				if number == 4 && (!hasElementScope(elements, "slideLayout") || !hasElementScope(elements, "slideMaster")) {
					t.Fatal("selected slide lost inherited layout or master content")
				}
			}
		})
	}
}

func TestSlidePreviewSelectedOfficeRenderKeepsChartsTablesAndInheritedContent(t *testing.T) {
	if _, err := lo.Find(); err != nil {
		t.Skipf("LibreOffice unavailable: %v", err)
	}
	file, _ := makeSlideSelectionDeck(t)
	out := filepath.Join(t.TempDir(), "preview")
	args := Args{Pos: []string{file}, Flags: map[string]string{"output": out, "slides": "2,3,4"}}
	result, err := runSlidePreview(context.Background(), args, convert.RenderFile)
	if err != nil {
		t.Fatal(err)
	}
	pairs := result.(map[string]any)["slides"].([]map[string]string)
	wantNumbers := []int{2, 3, 4}
	if len(pairs) != len(wantNumbers) {
		t.Fatalf("rendered %d selected pages, want %d", len(pairs), len(wantNumbers))
	}
	for i, number := range wantNumbers {
		if filepath.Base(pairs[i]["image"]) != fmt.Sprintf("slide-%d.png", number) {
			t.Fatalf("selected page %d has wrong output name: %s", number, pairs[i]["image"])
		}
		f, err := os.Open(pairs[i]["image"])
		if err != nil {
			t.Fatal(err)
		}
		_, err = png.DecodeConfig(f)
		f.Close()
		if err != nil {
			t.Fatalf("selected slide %d is not a rendered PNG: %v", number, err)
		}
	}
}

func hasElementKind(elements []any, kind string) bool {
	for _, element := range elements {
		if element.(map[string]any)["kind"] == kind {
			return true
		}
	}
	return false
}

func hasTableContent(elements []any) bool {
	for _, element := range elements {
		record := element.(map[string]any)
		cells, ok := record["cells"].([]any)
		if record["kind"] == "graphicFrame" && ok && len(cells) > 0 {
			return true
		}
	}
	return false
}

func hasElementScope(elements []any, scope string) bool {
	for _, element := range elements {
		if element.(map[string]any)["scope"] == scope {
			return true
		}
	}
	return false
}

func TestSlidePreviewSelectedSlideDoesNotParseUnselectedSlideXML(t *testing.T) {
	file, parts := makeSlideSelectionDeck(t)
	pkg, err := opc.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	pkg.Set(parts[1], []byte("not valid slide XML"))
	brokenUnselected := filepath.Join(filepath.Dir(file), "broken-unselected.pptx")
	if err := pkg.SaveNew(brokenUnselected); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "preview")
	render := previewFileRenderer(func(_ context.Context, source, renderDir, _, _ string, _ pdf.RenderOptions) ([]string, error) {
		snapshot, err := opc.Open(source)
		if err != nil {
			return nil, err
		}
		selected, err := office.PresentationSlides(snapshot)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(selected, parts[:1]) {
			return nil, fmt.Errorf("renderer saw %d slides, want selected slide 1", len(selected))
		}
		if err := os.MkdirAll(renderDir, 0755); err != nil {
			return nil, err
		}
		page := filepath.Join(renderDir, "only.png")
		if err := writePreview(page, image.NewRGBA(image.Rect(0, 0, 8, 6)), "png"); err != nil {
			return nil, err
		}
		return []string{page}, nil
	})
	args := Args{Pos: []string{brokenUnselected}, Flags: map[string]string{"output": out, "slides": "1"}}
	result, err := runSlidePreview(context.Background(), args, render)
	if err != nil {
		t.Fatal(err)
	}
	pairs := result.(map[string]any)["slides"].([]map[string]string)
	if len(pairs) != 1 || filepath.Base(pairs[0]["image"]) != "slide-1.png" {
		t.Fatalf("unexpected selected output: %#v", result)
	}
}

func TestSlidePreviewRejectsInvalidSelectionBeforeLayoutOrRender(t *testing.T) {
	file, originalParts := makeSlideSelectionDeck(t)
	pkg, err := opc.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	pkg.Remove(originalParts[len(originalParts)-1])
	missingLastSlide := filepath.Join(filepath.Dir(file), "missing-last-slide.pptx")
	if err := pkg.SaveNew(missingLastSlide); err != nil {
		t.Fatal(err)
	}
	file = missingLastSlide
	for _, selection := range []string{"0", "5", "2,,3"} {
		t.Run(selection, func(t *testing.T) {
			rendered := false
			out := filepath.Join(t.TempDir(), "preview")
			render := previewFileRenderer(func(context.Context, string, string, string, string, pdf.RenderOptions) ([]string, error) {
				rendered = true
				return nil, nil
			})
			args := Args{Pos: []string{file}, Flags: map[string]string{"output": out, "slides": selection}}
			if _, err := runSlidePreview(context.Background(), args, render); err == nil {
				t.Fatal("invalid selection accepted")
			}
			if rendered {
				t.Fatal("renderer called for invalid selection")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("invalid selection created output: %v", err)
			}
		})
	}
}
