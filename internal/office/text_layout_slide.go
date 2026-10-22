package office

import (
	"fmt"
	"github.com/beevik/etree"
	"math"
	"strings"
)

// SlideTextHeight returns the height in inches needed at the supplied width.
// Explicit run/default-run fonts are required; unresolved theme fonts fail.
func SlideTextHeight(shape *etree.Element, width float64) (float64, error) {
	body := Child(shape, NSP, "txBody")
	if body == nil {
		return 0, fmt.Errorf("fit_text requires a text shape")
	}
	bp := Child(body, NSA, "bodyPr")
	if v := AttrOr(bp, "vert", "horz"); v != "horz" {
		return 0, fmt.Errorf("fit_text requires horizontal text")
	}
	if emuAttr(Child(Child(shape, NSP, "spPr"), NSA, "xfrm"), "rot", 0) != 0 {
		return 0, fmt.Errorf("fit_text requires unrotated text")
	}
	available := width*72 - (emuAttr(bp, "lIns", 91440)+emuAttr(bp, "rIns", 91440))/12700
	total := (emuAttr(bp, "tIns", 45720) + emuAttr(bp, "bIns", 45720)) / 12700
	for _, par := range Children(body, NSA, "p") {
		if len(All(par, NSA, "fld")) > 0 {
			return 0, fmt.Errorf("fit_text cannot measure unresolved fields")
		}
		pr := Child(par, NSA, "pPr")
		// Hanging indents reserve room for bullets without flattening the content.
		usable := available - (emuAttr(pr, "marL", 0)+emuAttr(pr, "marR", 0)+math.Max(0, emuAttr(pr, "indent", 0)))/12700
		var runs []TextRun
		leadingBreaks := 0
		for _, c := range par.ChildElements() {
			if Is(c, NSA, "br") {
				if len(runs) > 0 {
					runs[len(runs)-1].Text += "\n"
				} else {
					leadingBreaks++
				}
				continue
			}
			if !Is(c, NSA, "r") {
				continue
			}
			holder := etree.NewElement("a:p")
			holder.CreateAttr("xmlns:a", NSA)
			if pr != nil {
				holder.AddChild(pr.Copy())
			}
			holder.AddChild(c.Copy())
			for _, r := range paragraphRuns(holder) {
				text := r.text
				if len(runs) == 0 {
					text = strings.Repeat("\n", leadingBreaks) + text
				}
				runs = append(runs, TextRun{text, r.family, r.size, r.bold, r.italic})
			}
		}
		if len(runs) == 0 {
			rpr := Child(par, NSA, "endParaRPr")
			if rpr == nil {
				rpr = Child(pr, NSA, "defRPr")
			}
			size, _ := ptAttr(rpr, "sz")
			runs = []TextRun{{Text: strings.Repeat("\n", leadingBreaks), Font: AttrOr(Child(rpr, NSA, "latin"), "typeface", ""), Size: size}}
		}
		h, err := TextHeight(runs, usable, AttrOr(bp, "wrap", "square") != "none")
		if err != nil {
			return 0, err
		}
		if ln := Child(pr, NSA, "lnSpc"); ln != nil {
			if pct := Child(ln, NSA, "spcPct"); pct != nil {
				h *= emuAttr(pct, "val", 100000) / 100000
			} else {
				return 0, fmt.Errorf("fit_text requires proportional line spacing")
			}
		}
		total += h
		if v, ok := spacingPt(pr, "spcBef"); ok {
			total += v
		}
		if v, ok := spacingPt(pr, "spcAft"); ok {
			total += v
		}
	}
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return 0, fmt.Errorf("invalid measured height")
	}
	return total / 72, nil
}
