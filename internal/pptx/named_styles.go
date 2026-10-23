package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strings"
)

// NamedTextStyle is a reusable spec-level style. PowerPoint stores the resolved
// formatting; names are not native PowerPoint styles or live theme bindings.
type NamedTextStyle struct {
	Description string   `json:"description,omitempty"`
	Font        string   `json:"font,omitempty"`
	FontSize    *float64 `json:"font_size,omitempty"`
	Color       string   `json:"color,omitempty"`
	Bold        *bool    `json:"bold,omitempty"`
	Italic      *bool    `json:"italic,omitempty"`
	Underline   *bool    `json:"underline,omitempty"`
	Align       string   `json:"align,omitempty"`
}

func (s NamedTextStyle) edit() Edit {
	return Edit{Font: s.Font, FontSize: s.FontSize, Color: s.Color, Bold: s.Bold, Italic: s.Italic, Underline: s.Underline}
}
func validateNamedStyles(styles map[string]NamedTextStyle) error {
	if len(styles) > 1000 {
		return fmt.Errorf("named_styles exceeds 1000 entries")
	}
	for name, s := range styles {
		if strings.TrimSpace(name) == "" || len(name) > 200 {
			return fmt.Errorf("style name must contain 1..200 bytes")
		}
		p := etree.NewElement("a:p")
		p.CreateAttr("xmlns:a", ans)
		if err := styleRunProperties(nil, "", p.CreateElement("a:rPr"), s.edit()); err != nil {
			return fmt.Errorf("style %q: %w", name, err)
		}
		if s.Align != "" {
			if err := styleParagraph(p, &ParagraphStyle{Align: s.Align}); err != nil {
				return fmt.Errorf("style %q: %w", name, err)
			}
		}
	}
	return nil
}
func applyNamedStyle(pkg *opc.Package, part string, shape *etree.Element, styles map[string]NamedTextStyle, name, scope string, paragraph, run int) error {
	s, ok := styles[name]
	if !ok || name == "" {
		return fmt.Errorf("unknown named style %q", name)
	}
	if scope == "" {
		scope = "shape"
	}
	if scope != "shape" && scope != "run" && scope != "paragraph" {
		return fmt.Errorf("style scope must be shape, paragraph or run")
	}
	body := office.Child(shape, pns, "txBody")
	if body == nil {
		return fmt.Errorf("named style requires a text shape")
	}
	paras := office.Children(body, ans, "p")
	if scope != "shape" {
		if paragraph < 0 || paragraph >= len(paras) {
			return fmt.Errorf("paragraph out of range")
		}
		paras = paras[paragraph : paragraph+1]
	}
	for _, p := range paras {
		if scope == "run" {
			if s.Align != "" {
				return fmt.Errorf("run style cannot set paragraph alignment; use paragraph or shape scope")
			}
			runs := office.Children(p, ans, "r")
			if run < 0 || run >= len(runs) {
				return fmt.Errorf("run out of range")
			}
			pr := office.Child(runs[run], ans, "rPr")
			if pr == nil {
				pr = etree.NewElement("a:rPr")
				runs[run].InsertChildAt(0, pr)
			}
			ensureDrawingPrefix(pr, runs[run])
			if err := styleRunProperties(pkg, part, pr, s.edit()); err != nil {
				return err
			}
			continue
		}
		pp := office.Child(p, ans, "pPr")
		if pp == nil {
			pp = etree.NewElement("a:pPr")
			p.InsertChildAt(0, pp)
		}
		ensureDrawingPrefix(pp, p)
		// Bind the new node before namespace-aware paragraph styling looks it up.
		if s.Align != "" {
			if err := styleParagraph(p, &ParagraphStyle{Align: s.Align}); err != nil {
				return err
			}
		}
		def := office.Child(pp, ans, "defRPr")
		if def == nil {
			def = etree.NewElement("a:defRPr")
			if ext := office.Child(pp, ans, "extLst"); ext != nil {
				pp.InsertChildAt(ext.Index(), def)
			} else {
				pp.AddChild(def)
			}
		}
		props := []*etree.Element{def}
		for _, r := range p.ChildElements() {
			if office.Is(r, ans, "r") || office.Is(r, ans, "fld") || office.Is(r, ans, "br") {
				pr := office.Child(r, ans, "rPr")
				if pr == nil {
					pr = etree.NewElement("a:rPr")
					r.InsertChildAt(0, pr)
				}
				props = append(props, pr)
			}
		}
		props = append(props, office.Children(p, ans, "endParaRPr")...)
		for _, pr := range props {
			ensureDrawingPrefix(pr, p)
			if err := styleRunProperties(pkg, part, pr, s.edit()); err != nil {
				return err
			}
		}
	}
	return nil
}
