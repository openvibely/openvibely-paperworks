package pptx

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
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

func TestAuthoredAndInheritedTextShareParagraphAndRunFallback(t *testing.T) {
	bold, notBold := true, false
	paragraphs := []Paragraph{
		{Bullet: true, Level: 2, Text: "bullet paragraph fallback"},
		{Runs: []Run{
			{Text: "styled", Bold: &bold, Italic: true, Underline: true, FontSize: 27, Color: "AABBCC"},
			{Text: "explicitly regular", Bold: &notBold, FontSize: 13, Color: "DDEEFF"},
		}},
	}
	for _, inherited := range []bool{false, true} {
		mode := "authored"
		if inherited {
			mode = "inherited"
		}
		t.Run(mode, func(t *testing.T) {
			e := Element{
				Text:              "legacy text must yield to paragraphs",
				Paragraphs:        paragraphs,
				FontSize:          20,
				Color:             "112233",
				Bold:              true,
				Align:             "ctr",
				InheritFormatting: inherited,
			}
			body := authoredTextBody(e, "Arial", false)
			doc := etree.NewDocument()
			if err := doc.ReadFromString(`<root xmlns:a="` + ans + `">` + body + `</root>`); err != nil {
				t.Fatal(err)
			}
			root := doc.Root()
			ps := root.FindElements("a:p")
			if len(ps) != 2 {
				t.Fatalf("paragraphs: %d", len(ps))
			}
			runs := root.FindElements(".//a:r")
			if len(runs) != 3 {
				t.Fatalf("runs: %d", len(runs))
			}
			for i, want := range []string{"bullet paragraph fallback", "styled", "explicitly regular"} {
				if got := runs[i].SelectElement("a:t").Text(); got != want {
					t.Fatalf("run %d text %q != %q", i, got, want)
				}
			}
			if len(root.FindElements(".//a:buChar")) != 1 {
				t.Fatal("bullet paragraph lost")
			}
			firstPPr := ps[0].SelectElement("a:pPr")
			if firstPPr.SelectAttrValue("lvl", "") != "2" || firstPPr.SelectAttrValue("algn", "") != "ctr" {
				t.Fatal("bullet paragraph properties changed")
			}
			styled := runs[1].SelectElement("a:rPr")
			for key, want := range map[string]string{"b": "1", "i": "1", "u": "sng", "sz": "2700"} {
				if got := styled.SelectAttrValue(key, ""); got != want {
					t.Fatalf("styled run %s=%q, want %q", key, got, want)
				}
			}
			if styled.FindElement("a:solidFill/a:srgbClr").SelectAttrValue("val", "") != "AABBCC" {
				t.Fatal("styled run color changed")
			}
			regular := runs[2].SelectElement("a:rPr")
			if regular.SelectAttrValue("b", "") != "0" || regular.SelectAttrValue("sz", "") != "1300" || regular.FindElement("a:solidFill/a:srgbClr").SelectAttrValue("val", "") != "DDEEFF" {
				t.Fatal("explicit false or run formatting lost")
			}
			bodyPr := root.SelectElement("a:bodyPr")
			endParagraphs := root.FindElements(".//a:endParaRPr")
			if inherited {
				if len(bodyPr.Attr) != 0 || len(endParagraphs) != 0 {
					t.Fatal("inherited text received authored body defaults")
				}
				if styled.SelectAttrValue("lang", "") != "" || styled.SelectElement("a:latin") != nil || regular.SelectAttrValue("i", "") != "" || regular.SelectAttrValue("u", "") != "" {
					t.Fatal("inherited run received authored run defaults")
				}
			} else {
				if bodyPr.SelectAttrValue("wrap", "") != "square" || len(endParagraphs) != 2 {
					t.Fatal("authored body defaults changed")
				}
				if styled.SelectAttrValue("lang", "") != "en-US" || styled.SelectElement("a:latin") == nil || regular.SelectAttrValue("i", "") != "0" || regular.SelectAttrValue("u", "") != "none" {
					t.Fatal("authored run defaults changed")
				}
			}

			legacy := Element{Text: "legacy first\nlegacy second", InheritFormatting: inherited}
			legacyDoc := etree.NewDocument()
			if err := legacyDoc.ReadFromString(`<root xmlns:a="` + ans + `">` + authoredTextBody(legacy, "Arial", false) + `</root>`); err != nil {
				t.Fatal(err)
			}
			legacyRuns := legacyDoc.Root().FindElements(".//a:r")
			if len(legacyDoc.Root().FindElements("a:p")) != 2 || len(legacyRuns) != 2 || legacyRuns[0].SelectElement("a:t").Text() != "legacy first" || legacyRuns[1].SelectElement("a:t").Text() != "legacy second" {
				t.Fatal("legacy text was not split into paragraphs")
			}
			legacyRPr := legacyRuns[0].SelectElement("a:rPr")
			if inherited {
				if legacyRPr.SelectAttrValue("sz", "") != "" || legacyRPr.SelectElement("a:solidFill") != nil {
					t.Fatal("inherited legacy text received authored font defaults")
				}
			} else if legacyRPr.SelectAttrValue("sz", "") != "1800" || legacyRPr.FindElement("a:solidFill/a:srgbClr").SelectAttrValue("val", "") != "172B4D" {
				t.Fatal("authored legacy text defaults changed")
			}
		})
	}
}
