package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestTextBoxControls(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	left := 12.0
	if err := Create(ctx, Spec{Slides: []Slide{{Elements: []Element{{Type: "text", X: 1, Y: 1, W: 4, H: 2, Text: "Keep this", TextBox: &TextBoxStyle{Insets: &TextInsets{Left: &left}, VerticalAlign: "middle", Wrap: "none", AutoFit: "shrinkText"}}}}}}, dir, in); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(in)
	d, _ := p.XML("ppt/slides/slide1.xml")
	pr := office.All(d.Root(), ans, "bodyPr")[0]
	if pr.SelectAttrValue("lIns", "") != "152400" || pr.SelectAttrValue("anchor", "") != "ctr" || pr.SelectAttrValue("wrap", "") != "none" || office.Child(pr, ans, "normAutofit") == nil {
		t.Fatal("creation did not apply text box")
	}
	for _, mode := range []string{"none", "resizeShapeToFitText"} {
		out := filepath.Join(dir, mode+".pptx")
		if _, err := EditFile(ctx, in, out, EditSpec{Edits: []Edit{{Operation: "text-box", Slide: 1, ShapeID: "2", TextBox: &TextBoxStyle{AutoFit: mode}}}}); err != nil {
			t.Fatal(err)
		}
		p, _ = opc.Open(out)
		d, _ = p.XML("ppt/slides/slide1.xml")
		pr = office.All(d.Root(), ans, "bodyPr")[0]
		tag := map[string]string{"none": "noAutofit", "resizeShapeToFitText": "spAutoFit"}[mode]
		if office.Child(pr, ans, tag) == nil || office.Child(pr, ans, "normAutofit") != nil || pr.SelectAttrValue("lIns", "") != "152400" || office.All(d.Root(), ans, "t")[0].Text() != "Keep this" {
			t.Fatal("edit lost text/settings or retained conflicting autofit")
		}
	}
	bad := -1.0
	for _, s := range []*TextBoxStyle{{Wrap: "bad"}, {VerticalAlign: "bad"}, {AutoFit: "bad"}, {Insets: &TextInsets{Top: &bad}}} {
		if _, err := EditFile(ctx, in, filepath.Join(dir, "bad.pptx"), EditSpec{Edits: []Edit{{Operation: "text-box", Slide: 1, ShapeID: "2", TextBox: s}}}); err == nil {
			t.Fatal("invalid setting accepted")
		}
	}
}
