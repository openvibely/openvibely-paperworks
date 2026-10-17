package pptx

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestEditableRunsAndPreservation(t *testing.T) {
	off := false
	s := Spec{Slides: []Slide{{Elements: []Element{{Type: "text", X: 1, Y: 1, W: 8, H: 2, Bold: true, Color: "123456", FontSize: 24, Paragraphs: []Paragraph{
		{Bullet: true, Runs: []Run{{Text: "Inherited "}, {Text: "<&> résumé", Bold: &off, Italic: true, Underline: true, Color: "FF0000", FontSize: 32}}},
		{Text: "Legacy paragraph"},
	}}}}}}
	in := filepath.Join(t.TempDir(), "runs.pptx")
	if err := Create(context.Background(), s, ".", in); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(in)
	p, err := opc.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("ppt/slides/slide1.xml")
	if err != nil {
		t.Fatal(err)
	}
	runs := d.FindElements("//a:r")
	if len(runs) != 3 {
		t.Fatalf("runs: %d", len(runs))
	}
	for i, want := range []string{"Inherited ", "<&> résumé", "Legacy paragraph"} {
		if got := runs[i].SelectElement("t").Text(); got != want {
			t.Fatalf("text %q != %q", got, want)
		}
	}
	for i, attrs := range []map[string]string{{"b": "1", "sz": "2400", "i": "0", "u": "none"}, {"b": "0", "sz": "3200", "i": "1", "u": "sng"}, {"b": "1", "sz": "2400"}} {
		pr := runs[i].SelectElement("rPr")
		for key, want := range attrs {
			if pr.SelectAttrValue(key, "") != want {
				t.Fatalf("run %d %s", i, key)
			}
		}
		ink := "123456"
		if i == 1 {
			ink = "FF0000"
		}
		if pr.FindElement("a:solidFill/a:srgbClr").SelectAttrValue("val", "") != ink {
			t.Fatal("color")
		}
	}
	if len(d.FindElements("//a:buChar")) != 1 {
		t.Fatal("native bullet lost")
	}
	out := filepath.Join(t.TempDir(), "notes.pptx")
	if _, err := EditFile(context.Background(), in, out, EditSpec{Edits: []Edit{{Slide: 1, Operation: "notes", Text: "Reviewed"}}}); err != nil {
		t.Fatal(err)
	}
	q, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if part.Name == "ppt/slides/slide1.xml" {
			for _, other := range q.Parts {
				if other.Name == part.Name && !bytes.Equal(part.Data, other.Data) {
					t.Fatal("notes edit changed runs")
				}
			}
		}
	}
	after, _ := os.ReadFile(in)
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
}

func TestRunValidation(t *testing.T) {
	for _, p := range []Paragraph{{Text: "ambiguous", Runs: []Run{{Text: "other"}}}, {Runs: []Run{{Color: "red"}}}, {Runs: []Run{{FontSize: -1}}}, {Runs: []Run{{FontSize: 401}}}, {Runs: []Run{{FontSize: math.NaN()}}}, {Runs: []Run{{FontSize: math.Inf(1)}}}, {Runs: make([]Run, 10001)}} {
		out := filepath.Join(t.TempDir(), "invalid.pptx")
		s := Spec{Slides: []Slide{{Elements: []Element{{Type: "text", X: 1, Y: 1, W: 2, H: 2, Paragraphs: []Paragraph{p}}}}}}
		if err := Create(context.Background(), s, ".", out); err == nil {
			t.Fatal("invalid runs accepted")
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("invalid output exists", err)
		}
	}
}
