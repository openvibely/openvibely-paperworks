package pdf

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// FixedParagraph is a single-line, plain ASCII paragraph in a translated frame.
// Position is the frame's lower-left corner. Baseline is Leading-FontSize above
// it. Wrapping, markup and inherited text-box settings are deliberately absent.
type FixedParagraph struct {
	Position [2]float64 `json:"pos"`
	Width    float64    `json:"width"`
	Text     string     `json:"text"`
	Font     string     `json:"font"`
	FontSize float64    `json:"font_size"`
	Leading  float64    `json:"leading"`
	Align    string     `json:"align,omitempty"`
}

func (p FixedParagraph) validate(ctx context.Context) error {
	for _, v := range []float64{p.Position[0], p.Position[1], p.Width, p.FontSize, p.Leading} {
		if !finite(v) {
			return fmt.Errorf("nonfinite paragraph geometry")
		}
	}
	if p.Width <= 0 || p.FontSize <= 0 || p.FontSize > 1000 || p.Leading < p.FontSize {
		return fmt.Errorf("invalid paragraph geometry")
	}
	if p.Font != "Helvetica" && p.Font != "Helvetica-Bold" {
		return fmt.Errorf("unsupported fixed paragraph font")
	}
	if p.Align != "" && p.Align != "left" && p.Align != "center" && p.Align != "right" {
		return fmt.Errorf("unsupported paragraph alignment")
	}
	if err := supportedText(p.Text); err != nil {
		return err
	}
	if strings.Join(strings.Fields(p.Text), " ") != p.Text || p.Text == "" {
		return fmt.Errorf("paragraph requires nonempty single-spaced text")
	}
	w, err := font.TextWidthFloat(ctx, p.Text, p.Font, p.FontSize)
	if err != nil {
		return err
	}
	if w > p.Width {
		return fmt.Errorf("fixed paragraph exceeds frame width")
	}
	return nil
}
func (p FixedParagraph) paint(ctx context.Context, b *bytes.Buffer, name string) error {
	w, err := font.TextWidthFloat(ctx, p.Text, p.Font, p.FontSize)
	if err != nil {
		return err
	}
	dx := 0.
	if p.Align == "center" {
		dx = (p.Width - w) / 2
	}
	if p.Align == "right" {
		dx = p.Width - w
	}
	n := reportNumber
	fmt.Fprintf(b, "q\n1 0 0 1 %s %s cm\nq\n0 0 0 rg\nBT 1 0 0 1 0 %s Tm /%s %s Tf %s TL ", n(p.Position[0]), n(p.Position[1]), n(p.Leading-p.FontSize), name, n(p.FontSize), n(p.Leading))
	if dx != 0 {
		fmt.Fprintf(b, "%s 0 Td ", n(dx))
	}
	fmt.Fprintf(b, "(%s) Tj T* ", fixedString(p.Text))
	if dx != 0 {
		fmt.Fprintf(b, "%s 0 Td ", n(-dx))
	}
	fmt.Fprint(b, "ET\nQ\nQ\n")
	return nil
}

// fixedFont reuses only an unmodified built-in WinAnsi resource.
func fixedFont(c *model.Context, fonts types.Dict, base string) string {
	for _, key := range sortedFontKeys(fonts) {
		d := dictOf(c.XRefTable, fonts[key]).d
		if len(d) == 4 && d["Type"] == types.Name("Font") && d["Subtype"] == types.Name("Type1") && d["BaseFont"] == types.Name(base) && d["Encoding"] == types.Name("WinAnsiEncoding") {
			return key
		}
	}
	name := "F1"
	for i := 2; fonts[name] != nil; i++ {
		name = fmt.Sprintf("F%d", i)
	}
	fonts[name] = types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type1"), "BaseFont": types.Name(base), "Encoding": types.Name("WinAnsiEncoding")}
	return name
}

func sortedFontKeys(d types.Dict) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
