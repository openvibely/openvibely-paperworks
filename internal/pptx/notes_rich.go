package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// replaceRichNotes changes paragraphs while retaining the notes placeholder and
// its body properties, so slide-number and thumbnail placeholders remain intact.
func replaceRichNotes(pkg *opc.Package, part string, body *etree.Element, paragraphs []Paragraph, font string) error {
	if len(paragraphs) > 10000 {
		return fmt.Errorf("notes paragraph limit exceeded")
	}
	for _, p := range paragraphs {
		if p.Text != "" && len(p.Runs) > 0 {
			return fmt.Errorf("paragraph text and runs are mutually exclusive")
		}
		if p.Level < 0 || p.Level > 8 || len(p.Runs) > 10000 {
			return fmt.Errorf("invalid notes paragraph")
		}
		for _, r := range p.Runs {
			if err := validateRunUnderline(r); err != nil {
				return err
			}
			if r.Style != "" {
				return fmt.Errorf("named run styles are supported for slide text, not notes")
			}
			if !finite(r.FontSize) || r.FontSize < 0 || r.FontSize > 400 {
				return fmt.Errorf("invalid run font size")
			}
			if _, err := color(r.Color, ""); err != nil {
				return err
			}
		}
	}
	if font == "" {
		font = "Arial"
	}
	d := etree.NewDocument()
	if err := d.ReadFromString("<p:txBody" + ns + ">" + textBody(Element{Paragraphs: paragraphs, FontSize: 12}, font) + "</p:txBody>"); err != nil {
		return err
	}
	if err := applyStructuredRunProperties(pkg, part, d.Root(), paragraphs); err != nil {
		return err
	}
	ps := office.Children(d.Root(), ans, "p")
	for i, p := range paragraphs {
		if p.Format != nil {
			if err := styleParagraph(ps[i], p.Format); err != nil {
				return err
			}
		}
	}
	for _, p := range office.Children(body, ans, "p") {
		body.RemoveChild(p)
	}
	for _, p := range ps {
		body.AddChild(p.Copy())
	}
	return nil
}
