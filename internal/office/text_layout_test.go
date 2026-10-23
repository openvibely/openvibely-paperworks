package office

import (
	"testing"

	"github.com/beevik/etree"
)

func TestTextHeightWrapsRunsAndNewlines(t *testing.T) {
	r := TextRun{Text: "A long sentence with words that must wrap", Font: "Go", Size: 12}
	wide, err := TextHeight([]TextRun{r}, 400, true)
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := TextHeight([]TextRun{r}, 60, true)
	if err != nil || narrow <= wide {
		t.Fatal(narrow, wide, err)
	}
	split := []TextRun{{Text: "A long sentence ", Font: "Go", Size: 12}, {Text: "with words that must wrap", Font: "Go", Size: 12}}
	h, err := TextHeight(split, 60, true)
	if err != nil || h != narrow {
		t.Fatal(h, narrow, err)
	}
	h, err = TextHeight([]TextRun{{Text: "A\nB\n", Font: "Go", Size: 12}}, 400, true)
	if err != nil || h != 3*wide {
		t.Fatal(h, wide, err)
	}
	if _, err = TextHeight([]TextRun{{Text: "X", Font: "missing-parity-font", Size: 12}}, 100, true); err == nil {
		t.Fatal("silently substituted a font")
	}
}

func TestSlideTextHeightCountsLeadingBreak(t *testing.T) {
	makeShape := func(br string) *etree.Element {
		d := etree.NewDocument()
		if err := d.ReadFromString(`<p:sp xmlns:p="` + NSP + `" xmlns:a="` + NSA + `"><p:txBody><a:bodyPr/><a:p>` + br + `<a:r><a:rPr sz="1200"><a:latin typeface="Go"/></a:rPr><a:t>Text</a:t></a:r></a:p></p:txBody></p:sp>`); err != nil {
			t.Fatal(err)
		}
		return d.Root()
	}
	a, err := SlideTextHeight(makeShape(""), 3)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SlideTextHeight(makeShape("<a:br/>"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if b <= a+.1 {
		t.Fatal("leading break lost", a, b)
	}
}
