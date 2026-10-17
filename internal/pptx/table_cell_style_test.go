package pptx

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Pinned reference presentation library leaves newly authored cell fills and run bold unset.
// Explicit user formatting must not be replaced by an automatic header palette.
func TestAuthoredTableCellStyleOverrides(t *testing.T) {
	for i := 0; i < 50; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			rows := make([][]string, 2+i%5)
			for r := range rows {
				rows[r] = make([]string, 1+i%4)
				for c := range rows[r] {
					rows[r][c] = fmt.Sprintf("cell %d %d", r, c)
				}
			}
			e := Element{Type: "table", W: 6, H: 3, Rows: rows, FontSize: 18}
			explicit := i >= 25
			if explicit {
				e.Fill = "112233"
				e.Color = "445566"
				e.Bold = true
			}
			dst := filepath.Join(t.TempDir(), "table.pptx")
			if err := Create(context.Background(), Spec{Slides: []Slide{{Elements: []Element{e}}}}, ".", dst); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(dst)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("ppt/slides/slide1.xml")
			if err != nil {
				t.Fatal(err)
			}
			cells := office.All(d.Root(), ans, "tc")
			if len(cells) != len(rows)*len(rows[0]) {
				t.Fatal("cell count")
			}
			for _, cell := range cells {
				props := office.Child(cell, ans, "tcPr")
				fills := office.All(props, ans, "srgbClr")
				if explicit {
					if len(fills) != 1 || fills[0].SelectAttrValue("val", "") != "112233" {
						t.Fatal("explicit fill lost")
					}
				} else if len(props.ChildElements()) != 0 {
					t.Fatal("unexpected cell override")
				}
				runs := office.All(cell, ans, "rPr")
				if len(runs) != 1 {
					t.Fatal("run count")
				}
				want := ""
				color := "172B4D"
				if explicit {
					want = "1"
					color = "445566"
				}
				if runs[0].SelectAttrValue("b", "") != want {
					t.Fatal("bold override")
				}
				ink := office.All(runs[0], ans, "srgbClr")
				if len(ink) != 1 || ink[0].SelectAttrValue("val", "") != color {
					t.Fatal("text color override")
				}
			}
		})
	}
}
