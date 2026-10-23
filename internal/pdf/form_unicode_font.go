package pdf

import (
	"context"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/primitives"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

func formUnicodeFont() (*sfnt.Font, string, error) {
	parsed, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		return nil, "", fmt.Errorf("load embedded form font: %w", err)
	}
	name, err := parsed.Name(nil, sfnt.NameIDPostScript)
	if err != nil || name == "" {
		return nil, "", fmt.Errorf("embedded form font has no PostScript name")
	}
	return parsed, name, nil
}

func validateFormText(s string, parsed *sfnt.Font, name string, buf *sfnt.Buffer) (bool, error) {
	if !utf8.ValidString(s) {
		return false, fmt.Errorf("form text is not valid UTF-8")
	}
	hasUnicode := false
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' && r != '\v' && r != '\f' {
			return false, fmt.Errorf("unsupported control character U+%04X in form text", r)
		}
		if r <= 126 && r >= 32 || r == '\n' || r == '\r' || r == '\t' || r == '\v' || r == '\f' {
			continue
		}
		hasUnicode = true
		glyph, err := parsed.GlyphIndex(buf, r)
		if err != nil || glyph == 0 {
			return false, fmt.Errorf("unsupported character U+%04X: embedded font %q has no glyph", r, name)
		}
	}
	return hasUnicode, nil
}

func withFormUnicodeFont(current **primitives.FormFont, name string) {
	font := &primitives.FormFont{Name: name, Size: 12}
	if *current != nil {
		*font = **current
		font.Name = name
		if font.Size <= 0 {
			font.Size = 12
		}
	}
	*current = font
}

func formHasEmbeddedUnicodeFont(ctx context.Context, x *model.XRefTable, name string) bool {
	resources, err := primitives.FormFontResDict(x)
	if err != nil || resources == nil {
		return false
	}
	for _, object := range resources {
		ref, ok := object.(types.IndirectRef)
		if !ok {
			continue
		}
		fontName, _, _, err := primitives.FormFontDetails(ctx, x, ref)
		if err == nil && fontName == name {
			return true
		}
	}
	return false
}

func installFormUnicodeFont(fontDir string) error {
	if err := font.InstallFontFromBytesQuiet(fontDir, "form-unicode.ttf", goregular.TTF); err != nil {
		return fmt.Errorf("install embedded form font: %w", err)
	}
	return nil
}
