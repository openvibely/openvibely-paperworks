package xlsx

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Replace the whole color element: RGB must not retain theme/indexed/tint.
func setTabColor(f *excelize.File, name string, color *string) (*excelize.File, error) {
	if color == nil {
		return nil, fmt.Errorf("color required; use an empty string to clear")
	}
	if *color != "" {
		if len(*color) != 6 {
			return nil, fmt.Errorf("color must be six hexadecimal RGB digits or empty")
		}
		if _, err := hex.DecodeString(*color); err != nil {
			return nil, fmt.Errorf("invalid tab color: %w", err)
		}
	}
	idx, err := f.GetSheetIndex(name)
	if err != nil {
		return nil, err
	}
	if idx < 0 {
		return nil, fmt.Errorf("sheet %q not found", name)
	}
	raw, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	p, err := opc.Read(raw.Bytes())
	if err != nil {
		return nil, err
	}
	w, err := office.OpenWorkbook(p)
	if err != nil {
		return nil, err
	}
	sheet := w.Sheets[f.GetSheetName(idx)]
	if sheet == nil {
		return nil, fmt.Errorf("worksheet %q not found", name)
	}
	root := sheet.Doc.Root()
	props := office.Child(root, office.NSS, "sheetPr")
	if props == nil && *color != "" {
		props = etree.NewElement("sheetPr")
		props.Space = root.Space
		root.InsertChildAt(0, props)
	}
	if props != nil {
		for _, child := range props.ChildElements() {
			if child.Tag == "tabColor" && child.NamespaceURI() == office.NSS {
				props.RemoveChild(child)
			}
		}
		if *color != "" {
			c := etree.NewElement("tabColor")
			c.Space = props.Space
			c.CreateAttr("rgb", "FF"+strings.ToUpper(*color))
			props.InsertChildAt(0, c)
		}
	}
	if err = p.SetXML(sheet.Part, sheet.Doc); err != nil {
		return nil, err
	}
	data, err := p.Bytes()
	if err != nil {
		return nil, err
	}
	return excelize.OpenReader(bytes.NewReader(data))
}
