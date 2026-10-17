package pptx

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestInchesTruncateLikePinnedReference(t *testing.T) {
	// Expected values acquired from unchanged reference presentation library 1.0.2 Inches.
	for _, tc := range []struct {
		inches float64
		want   int64
	}{
		{13.333, 12191695}, {13.333333, 12191999}, {7.5, 6858000},
		{.7, 640080}, {.3, 274320}, {12, 10972800}, {1.6, 1463040},
		{7, 6400800}, {4.8, 4389120}, {8.2, 7498079}, {2, 1828800},
		{4.3, 3931920}, {.000001, 0}, {.0000009, 0},
	} {
		if got := emu(tc.inches); got != tc.want {
			t.Fatalf("Inches(%g) = %d; want %d", tc.inches, got, tc.want)
		}
	}
	out := filepath.Join(t.TempDir(), "units.pptx")
	err := Create(context.Background(), Spec{Width: 13.333, Height: 7.5, Slides: []Slide{{Elements: []Element{{Type: "text", X: 8.2, Y: .3, W: 4.3, H: 1.6, Text: "Geometry"}}}}}, "", out)
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		part, ns, tag, attr string
		want                int64
	}{
		{"ppt/presentation.xml", pns, "sldSz", "cx", 12191695},
		{"ppt/presentation.xml", pns, "sldSz", "cy", 6858000},
		{"ppt/slides/slide1.xml", ans, "off", "x", 7498079},
		{"ppt/slides/slide1.xml", ans, "off", "y", 274320},
		{"ppt/slides/slide1.xml", ans, "ext", "cx", 3931920},
		{"ppt/slides/slide1.xml", ans, "ext", "cy", 1463040},
	} {
		d, err := p.XML(tc.part)
		if err != nil {
			t.Fatal(err)
		}
		nodes := office.All(d.Root(), tc.ns, tc.tag)
		// Group geometry precedes the authored shape in slide1.xml.
		if len(nodes) == 0 {
			t.Fatal("missing", tc.tag)
		}
		if got := nodes[len(nodes)-1].SelectAttrValue(tc.attr, ""); got != strconv.FormatInt(tc.want, 10) {
			t.Fatalf("%s %s %s=%s; want %d", tc.part, tc.tag, tc.attr, got, tc.want)
		}
	}
}
