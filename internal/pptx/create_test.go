package pptx

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/beevik/etree"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

func fixtureSpec(t *testing.T) Spec {
	t.Helper()
	data, e := os.ReadFile("../../testdata/workflows/revenue-deck.json")
	if e != nil {
		t.Fatal(e)
	}
	var s Spec
	if e = json.Unmarshal(data, &s); e != nil {
		t.Fatal(e)
	}
	return s
}
func createFixture(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "deck.pptx")
	if e := Create(context.Background(), fixtureSpec(t), ".", file); e != nil {
		t.Fatal(e)
	}
	return file
}
func TestNarrowEditsPreserveWorkbookAndOtherSlides(t *testing.T) {
	in := createFixture(t)
	raw, _ := os.ReadFile(in)
	out := filepath.Join(t.TempDir(), "edited.pptx")
	changed, e := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Slide: 2, ShapeID: "2", Operation: "text", Text: "Updated sample revenue"}, {Slide: 2, ShapeID: "4", Operation: "table-cell", Row: 1, Column: 0, Text: "Subscriptions (sample)"}, {Slide: 2, Operation: "notes", Text: "Reviewed projection.\nStill illustrative."}}})
	if e != nil {
		t.Fatal(e)
	}
	if len(changed) != 2 {
		t.Fatal(changed)
	}
	p, _ := opc.Open(in)
	q, _ := opc.Open(out)
	for _, part := range p.Parts {
		if part.Name == "ppt/slides/slide2.xml" || part.Name == "ppt/notesSlides/notesSlide2.xml" {
			continue
		}
		if !bytes.Equal(part.Data, q.Get(part.Name)) {
			t.Fatal("unrelated part changed", part.Name)
		}
	}
	for part, want := range map[string][]string{"ppt/slides/slide2.xml": {"Updated sample revenue", "Subscriptions (sample)"}, "ppt/notesSlides/notesSlide2.xml": {"Reviewed projection.", "Still illustrative."}} {
		for _, s := range want {
			if !bytes.Contains(q.Get(part), []byte(s)) {
				t.Fatal("missing edit", part, s)
			}
		}
	}
	after, _ := os.ReadFile(in)
	if !bytes.Equal(raw, after) {
		t.Fatal("source changed")
	}
	r, e := office.PackageReportOf(q)
	if e != nil {
		t.Fatal(e)
	}
	if issues := r["issues"].([]string); len(issues) > 0 {
		t.Fatal(issues)
	}
}
func TestCreationGuards(t *testing.T) {
	for _, kind := range []string{"geometry", "color", "series", "type", "table", "list"} {
		t.Run(kind, func(t *testing.T) {
			s := fixtureSpec(t)
			switch kind {
			case "geometry":
				s.Slides[0].Elements[0].W = 100
			case "color":
				s.Slides[0].Elements[0].Color = "not-a-color"
			case "series":
				s.Slides[1].Elements[1].Chart.Series[0].Values = nil
			case "type":
				s.Slides[0].Elements[0].Type = "unknown"
			case "table":
				s.Slides[1].Elements[2].Rows[0] = nil
			case "list":
				s.Slides[0].Elements[0].Paragraphs = []Paragraph{{Text: "x", Bullet: true, Level: 9}}
			}
			out := filepath.Join(t.TempDir(), "no.pptx")
			if e := Create(context.Background(), s, ".", out); e == nil {
				t.Fatal("invalid input accepted")
			}
			if _, e := os.Stat(out); !os.IsNotExist(e) {
				t.Fatal("failed creation left an output")
			}
		})
	}
	file := createFixture(t)
	before, _ := os.ReadFile(file)
	if e := Create(context.Background(), fixtureSpec(t), ".", file); e == nil {
		t.Fatal("overwrote output")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("conflicting output changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := Create(ctx, fixtureSpec(t), ".", filepath.Join(t.TempDir(), "no.pptx")); e != context.Canceled {
		t.Fatal(e)
	}
}
func TestLibreOfficeEightSlideAcceptance(t *testing.T) {
	if _, e := lo.Find(); e != nil {
		t.Skip("LibreOffice unavailable; native acceptance not run")
	}
	ctx := context.Background()
	in := createFixture(t)
	before, _ := os.ReadFile(in)
	result, e := convert.Convert(ctx, in, filepath.Join(t.TempDir(), "pdf"), convert.Options{})
	if e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(result["output"].(string))
	if e != nil {
		t.Fatal(e)
	}
	pages, e := pdf.PageText(ctx, data)
	if e != nil {
		t.Fatal(e)
	}
	if len(pages) != 8 {
		t.Fatal("wrong page count", len(pages))
	}
	for i, words := range map[int][]string{0: {"subscription-led", "FY2027"}, 1: {"Subscriptions", "Services", "120", "80"}, 3: {"FY2027", "FY2028", "FY2029", "Subscriptions", "Services", "Total"}, 4: {"172.8", "88.2", "261"}, 7: {"Approve validation"}} {
		for _, word := range words {
			if !strings.Contains(pages[i], word) {
				t.Fatalf("slide %d missing %q in application PDF: %s", i+1, word, pages[i])
			}
		}
	}
	// The native chart must paint colored data outside the title/table regions.
	_, e = pdf.RenderImages(ctx, data, pdf.RenderOptions{Pages: []int{2, 4}, MaxPixels: 1200}, func(page int, img *image.RGBA) error {
		colored := 0
		for y := img.Bounds().Dy() / 3; y < img.Bounds().Dy()*4/5; y++ {
			for x := img.Bounds().Dx() / 10; x < img.Bounds().Dx()/2; x++ {
				c := img.RGBAAt(x, y)
				max := max(c.R, c.G, c.B)
				min := min(c.R, c.G, c.B)
				if max-min > 50 && c.A > 200 {
					colored++
				}
			}
		}
		if colored < 100 {
			t.Errorf("page %d chart data region has only %d colored pixels", page, colored)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	// Native application resave and reopen is distinct from ZIP/XML validation.
	refreshed, e := convert.Convert(ctx, in, filepath.Join(t.TempDir(), "reopened"), convert.Options{Refresh: true})
	if e != nil {
		t.Fatal(e)
	}
	p, e := opc.Open(refreshed["output"].(string))
	if e != nil {
		t.Fatal(e)
	}
	slides, e := office.PresentationSlides(p)
	if e != nil || len(slides) != 8 {
		t.Fatal("application lost slides", slides, e)
	}
	notes := 0
	for _, part := range p.Parts {
		if strings.HasPrefix(part.Name, "ppt/notesSlides/") && strings.HasSuffix(part.Name, ".xml") && !strings.Contains(part.Name, "/_rels/") {
			notes++
		}
	}
	if notes != 8 {
		t.Fatal("application lost notes", notes)
	}
	var noteText strings.Builder
	for _, part := range p.Parts {
		if strings.HasPrefix(part.Name, "ppt/notesSlides/") && strings.HasSuffix(part.Name, ".xml") && !strings.Contains(part.Name, "/_rels/") {
			d, err := p.XML(part.Name)
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range office.All(d.Root(), office.NSA, "t") {
				noteText.WriteString(text.Text())
			}
		}
	}
	for _, slide := range fixtureSpec(t).Slides {
		if !strings.Contains(noteText.String(), slide.Notes) {
			t.Fatal("application changed speaker notes", slide.Notes)
		}
	}
	pres, err := p.XML("ppt/presentation.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(office.All(pres.Root(), office.NSP, "notesMasterId")) != 1 {
		t.Fatal("application lost notes master linkage")
	}
	parts, e := office.ChartParts(p)
	if e != nil || len(parts) != 2 {
		t.Fatal("application lost chart objects", parts, e)
	}
	original, _ := opc.Open(in)
	for _, part := range parts {
		got, e := p.XML(part)
		if e != nil {
			t.Fatal(e)
		}
		want, e := original.XML(part)
		if e != nil {
			t.Fatal(e)
		}
		values := func(d *etree.Document) []string {
			var out []string
			for _, cache := range office.All(d.Root(), office.NSC, "numCache") {
				for _, v := range office.All(cache, office.NSC, "v") {
					out = append(out, v.Text())
				}
			}
			return out
		}
		if !slices.Equal(values(got), values(want)) {
			t.Fatal("application changed chart values", part, values(got), values(want))
		}
	}
	if _, e := office.VerifyChartData(p, ""); e != nil {
		t.Log("LibreOffice round-trip limitation (copy only):", e, "; source embedded-workbook validation passed separately; resaved copy is not deliverable")
	}
	after, _ := os.ReadFile(in)
	if !bytes.Equal(before, after) {
		t.Fatal("application check changed source")
	}
	t.Log("Native application:", lo.Version(ctx), "; PDF text, chart pixels, and PPTX reopening checked. No human/model visual inspection.")
}
