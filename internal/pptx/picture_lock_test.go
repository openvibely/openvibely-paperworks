package pptx

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"path/filepath"
	"testing"
)

func TestPictureLockRoundTrip(t *testing.T) {
	in := pictureMaskFixture(t)
	for _, v := range []bool{true, false} {
		out := filepath.Join(t.TempDir(), "out.pptx")
		if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Operation: "image-lock", Slide: 1, ShapeID: "2", LockAspectRatio: &v}}}); err != nil {
			t.Fatal(err)
		}
		p, _ := opc.Open(out)
		d, _ := p.XML("ppt/slides/slide1.xml")
		ls := office.All(d.Root(), ans, "picLocks")
		want := "0"
		if v {
			want = "1"
		}
		if len(ls) != 1 || ls[0].SelectAttrValue("noChangeAspect", "") != want {
			t.Fatal("lock not saved")
		}
		if _, err := office.VerifyPreservation(in, out, []string{"ppt/slides/slide1.xml"}); err != nil {
			t.Fatal(err)
		}
		in = out
	}
}
func TestPictureLockCreation(t *testing.T) {
	source := pictureMaskFixture(t)
	v := true
	out := filepath.Join(t.TempDir(), "out.pptx")
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "image", Path: filepath.Join(filepath.Dir(source), "image.png"), W: 2, H: 1, LockAspectRatio: &v}}}}}, "", out); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("ppt/slides/slide1.xml")
	if office.All(d.Root(), ans, "picLocks")[0].SelectAttrValue("noChangeAspect", "") != "1" {
		t.Fatal("missing lock")
	}
	if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{{Type: "shape", W: 2, H: 1, LockAspectRatio: &v}}}}}, "", filepath.Join(t.TempDir(), "bad.pptx")); err == nil {
		t.Fatal("nonpicture lock accepted")
	}
}
