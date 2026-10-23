package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Replace only text paragraphs; cell borders, margins and body properties survive.
func replaceRichTableCell(pkg *opc.Package, part string, cell *etree.Element, paragraphs []Paragraph) error {
	body := office.Child(cell, ans, "txBody")
	if body == nil {
		return fmt.Errorf("cell has no text body")
	}
	// Use the existing validated structured-text writer. It retains bodyPr/lstStyle.
	if err := replaceRichNotes(pkg, part, body, paragraphs, "Arial"); err != nil {
		return err
	}
	for _, p := range office.Children(body, ans, "p") {
		ensureDrawingPrefix(p, body)
	}
	return nil
}
