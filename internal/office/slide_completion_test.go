package office

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// DocumentGapChecks.test_slide_completion_rejects_offslide_placeholder_heading_and_bullet.
func TestReferenceSlideCompletionFailures(t *testing.T) {
	for _, issue := range []string{"bounds", "Chart Title", "heading", "bullet"} {
		t.Run(issue, func(t *testing.T) {
			p := previewOpen(t, "../../testdata/reference-mirror/literal-chart.pptx")
			// Restore the original nonliteral chart; do not exercise the production
			// materializer to manufacture the test's input.
			chartBytes, err := os.ReadFile("../../testdata/reference-mirror/reference-column-chart.xml")
			if err != nil {
				t.Fatal(err)
			}
			p.Set("ppt/charts/chart1.xml", chartBytes)
			if issue == "Chart Title" {
				d, e := p.XML("ppt/charts/chart1.xml")
				if e != nil {
					t.Fatal(e)
				}
				chart := Child(d.Root(), NSC, "chart")
				title := NewChild(chart, NSC, "title", nil)
				chart.RemoveChild(title)
				chart.InsertChildAt(0, title)
				tx := NewChild(title, NSC, "tx", nil)
				rich := NewChild(tx, NSC, "rich", nil)
				NewChild(rich, NSA, "bodyPr", nil)
				NewChild(rich, NSA, "lstStyle", nil)
				para := NewChild(rich, NSA, "p", nil)
				run := NewChild(para, NSA, "r", nil)
				NewChild(run, NSA, "t", nil).SetText("Chart Title")
				NewChild(title, NSC, "layout", nil)
				NewChild(title, NSC, "overlay", nil).CreateAttr("val", "0")
				if e := p.SetXML("ppt/charts/chart1.xml", d); e != nil {
					t.Fatal(e)
				}
			} else {
				x, w, h := 0, 2743200, 274320
				text := `<a:r><a:t>First line</a:t></a:r><a:br/><a:r><a:t>Second line</a:t></a:r>`
				props := `<a:pPr><a:defRPr sz="3000"/></a:pPr>`
				autofit := `<a:noAutofit/>`
				if issue == "bounds" {
					x = 9144001
					w = 914400
					h = 914400
					text = `<a:r><a:t>Outside</a:t></a:r>`
					props = ""
					autofit = `<a:spAutoFit/>`
				}
				if issue == "bullet" {
					h = 1828800
					props = `<a:pPr marL="0" indent="0"><a:defRPr sz="3000"/><a:buChar char="•"/></a:pPr>`
				}
				shape := etree.NewDocument()
				if e := shape.ReadFromString(fmt.Sprintf(`<p:sp xmlns:p="%s" xmlns:a="%s"><p:nvSpPr><p:cNvPr id="3" name="TextBox 2"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="none">%s</a:bodyPr><a:lstStyle/><a:p>%s%s</a:p></p:txBody></p:sp>`, NSP, NSA, x, w, h, autofit, props, text)); e != nil {
					t.Fatal(e)
				}
				slide, e := p.XML("ppt/slides/slide1.xml")
				if e != nil {
					t.Fatal(e)
				}
				All(slide.Root(), NSP, "spTree")[0].AddChild(shape.Root().Copy())
				if e := p.SetXML("ppt/slides/slide1.xml", slide); e != nil {
					t.Fatal(e)
				}
			}
			saved := filepath.Join(t.TempDir(), "case.pptx")
			if err := p.SaveNew(saved); err != nil {
				t.Fatal(err)
			}
			p = previewOpen(t, saved)
			report, e := PackageReportOf(p)
			if e != nil {
				t.Fatal(e)
			}
			if _, e := VerifyPresentation(p, report, SlideRequirements{}); e == nil || !strings.Contains(e.Error(), issue) {
				t.Fatalf("want %q rejection, got %v", issue, e)
			}
		})
	}
}
