// Package docx authors new WordprocessingML packages from a JSON spec.
package docx

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/docxtemplate"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

type Run struct {
	Text      string  `json:"text"`
	Bold      bool    `json:"bold,omitempty"`
	Italic    bool    `json:"italic,omitempty"`
	Underline bool    `json:"underline,omitempty"`
	Color     string  `json:"color,omitempty"`
	Size      float64 `json:"size,omitempty"`
	Link      string  `json:"link,omitempty"`
}

type Block struct {
	Type        string     `json:"type"`
	Text        string     `json:"text,omitempty"`
	Runs        []Run      `json:"runs,omitempty"`
	Level       int        `json:"level,omitempty"`
	Style       string     `json:"style,omitempty"`
	Align       string     `json:"align,omitempty"`
	Rows        [][]string `json:"rows,omitempty"`
	Header      *bool      `json:"header,omitempty"`
	Autofit     bool       `json:"autofit,omitempty"`
	HeaderBold  *bool      `json:"header_bold,omitempty"`
	Widths      []int      `json:"widths_twips,omitempty"`
	CellAlign   [][]string `json:"cell_align,omitempty"`
	CellMargin  int        `json:"cell_margin_twips,omitempty"`
	Items       []string   `json:"items,omitempty"`
	Ordered     bool       `json:"ordered,omitempty"`
	Path        string     `json:"path,omitempty"`
	WidthIn     float64    `json:"width_in,omitempty"`
	Description string     `json:"description,omitempty"`
}

type Page struct {
	WidthIn   float64 `json:"width_in"`
	HeightIn  float64 `json:"height_in"`
	MarginIn  float64 `json:"margin_in"`
	Landscape bool    `json:"landscape"`
}

type Spec struct {
	CompatibilityMode int     `json:"compatibility_mode,omitempty"`
	StylesFile        string  `json:"styles_file,omitempty"`
	StylesOverlayFile string  `json:"styles_overlay_file,omitempty"`
	ThemeFile         string  `json:"theme_file,omitempty"`
	Page              Page    `json:"page"`
	Font              string  `json:"font"`
	FontSize          float64 `json:"font_size"`
	Title             string  `json:"title"`
	Author            string  `json:"author"`
	Header            string  `json:"header"`
	Footer            string  `json:"footer"`
	PageNumbers       bool    `json:"page_numbers"`
	Blocks            []Block `json:"blocks"`
}

const (
	NSW    = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	ctMain = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
)

func esc(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

type builder struct {
	spec             Spec
	pkg              *opc.Package
	body             strings.Builder
	docRels          []string
	relN             int
	imgN             int
	numbered         bool
	bulleted         bool
	orderedListCount int
	base             string
}

func (b *builder) rel(typ, target string, external bool) string {
	b.relN++
	id := fmt.Sprintf("rId%d", b.relN)
	mode := ""
	if external {
		mode = ` TargetMode="External"`
	}
	b.docRels = append(b.docRels, fmt.Sprintf(`<Relationship Id="%s" Type="%s/%s" Target="%s"%s/>`, id, opc.NSOffR, typ, esc(target), mode))
	return id
}

func runXML(r Run) string {
	var pr strings.Builder
	if r.Bold {
		pr.WriteString("<w:b/>")
	}
	if r.Italic {
		pr.WriteString("<w:i/>")
	}
	if r.Color != "" {
		fmt.Fprintf(&pr, `<w:color w:val="%s"/>`, esc(strings.TrimPrefix(r.Color, "#")))
	}
	if r.Size > 0 {
		fmt.Fprintf(&pr, `<w:sz w:val="%d"/>`, int(r.Size*2))
	}
	if r.Underline {
		pr.WriteString(`<w:u w:val="single"/>`)
	}
	var out strings.Builder
	out.WriteString("<w:r>")
	if pr.Len() > 0 {
		out.WriteString("<w:rPr>" + pr.String() + "</w:rPr>")
	}
	for i, line := range strings.Split(strings.ReplaceAll(r.Text, "\r", "\n"), "\n") {
		if i > 0 {
			out.WriteString("<w:br/>")
		}
		for j, part := range strings.Split(line, "\t") {
			if j > 0 {
				out.WriteString("<w:tab/>")
			}
			if part != "" {
				space := ""
				if strings.TrimSpace(part) != part {
					space = ` xml:space="preserve"`
				}
				fmt.Fprintf(&out, `<w:t%s>%s</w:t>`, space, esc(part))
			}
		}
	}
	out.WriteString("</w:r>")
	return out.String()
}

func (b *builder) runs(blk Block) string {
	runs := blk.Runs
	if len(runs) == 0 && blk.Text != "" {
		runs = []Run{{Text: blk.Text}}
	}
	var out strings.Builder
	for _, r := range runs {
		if r.Link != "" {
			id := b.rel("hyperlink", r.Link, true)
			r.Underline, r.Color = true, firstNonEmpty(r.Color, "0563C1")
			fmt.Fprintf(&out, `<w:hyperlink r:id="%s">%s</w:hyperlink>`, id, runXML(r))
			continue
		}
		out.WriteString(runXML(r))
	}
	return out.String()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func para(style, align, extraPPr, content string) string {
	var pr strings.Builder
	if style != "" {
		fmt.Fprintf(&pr, `<w:pStyle w:val="%s"/>`, esc(style))
	}
	pr.WriteString(extraPPr)
	if align != "" {
		fmt.Fprintf(&pr, `<w:jc w:val="%s"/>`, esc(align))
	}
	if pr.Len() == 0 {
		return "<w:p>" + content + "</w:p>"
	}
	return "<w:p><w:pPr>" + pr.String() + "</w:pPr>" + content + "</w:p>"
}

var alignMap = map[string]string{"": "", "left": "left", "center": "center", "right": "right", "justify": "both", "both": "both"}

func (b *builder) block(blk Block) error {
	align, ok := alignMap[blk.Align]
	if !ok {
		return fmt.Errorf("unsupported alignment %q", blk.Align)
	}
	switch blk.Type {
	case "heading":
		if blk.Level < 0 || blk.Level > 6 {
			return errors.New("heading level must be 0–6")
		}
		style := "Title"
		if blk.Level > 0 {
			style = fmt.Sprintf("Heading%d", blk.Level)
		}
		b.body.WriteString(para(style, align, "", b.runs(blk)))
	case "paragraph", "":
		b.body.WriteString(para(blk.Style, align, "", b.runs(blk)))
	case "list":
		num := 2
		if blk.Ordered {
			num = 1
			if b.orderedListCount > 0 {
				num = b.orderedListCount + 2
			}
			b.orderedListCount++
			b.numbered = true
		} else {
			b.bulleted = true
		}
		for _, it := range blk.Items {
			b.body.WriteString(para("ListParagraph", align, fmt.Sprintf(`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="%d"/></w:numPr>`, num), runXML(Run{Text: it})))
		}
	case "page_break":
		b.body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
	case "table":
		return b.table(blk)
	case "image":
		return b.image(blk, align)
	default:
		return fmt.Errorf("unsupported block type %q", blk.Type)
	}
	return nil
}

func (b *builder) usableTwips() int {
	w, m := b.spec.Page.WidthIn, b.spec.Page.MarginIn
	if b.spec.Page.Landscape {
		w = b.spec.Page.HeightIn
	}
	return int((w - 2*m) * 1440)
}

func (b *builder) table(blk Block) error {
	if len(blk.Rows) == 0 {
		return errors.New("table needs rows")
	}
	cols := len(blk.Rows[0])
	for _, r := range blk.Rows {
		if len(r) != cols || cols == 0 {
			return errors.New("table rows must be rectangular")
		}
	}
	widths := blk.Widths
	if widths == nil {
		for range cols {
			widths = append(widths, b.usableTwips()/cols)
		}
	}
	if len(widths) != cols {
		return errors.New("widths_twips must match the column count")
	}
	total := 0
	for _, w := range widths {
		total += w
	}
	header := blk.Header == nil || *blk.Header
	headerBold := blk.HeaderBold == nil || *blk.HeaderBold
	style := firstNonEmpty(blk.Style, "TableGrid")
	margin := blk.CellMargin
	if margin == 0 {
		margin = 100
	}
	var t strings.Builder
	widthAndLayout := fmt.Sprintf(`<w:tblW w:w="%d" w:type="dxa"/><w:tblLayout w:type="fixed"/>`, total)
	if blk.Autofit {
		widthAndLayout = `<w:tblW w:type="auto" w:w="0"/>`
	}
	fmt.Fprintf(&t, `<w:tbl><w:tblPr><w:tblStyle w:val="%s"/>%s<w:tblLook w:firstColumn="1" w:firstRow="1" w:lastColumn="0" w:lastRow="0" w:noHBand="0" w:noVBand="1" w:val="04A0"/></w:tblPr><w:tblGrid>`, esc(style), widthAndLayout)
	for _, w := range widths {
		fmt.Fprintf(&t, `<w:gridCol w:w="%d"/>`, w)
	}
	t.WriteString("</w:tblGrid>")
	for i, row := range blk.Rows {
		t.WriteString("<w:tr>")
		if i == 0 && header {
			t.WriteString("<w:trPr><w:tblHeader/></w:trPr>")
		}
		for j, cell := range row {
			fmt.Fprintf(&t, `<w:tc><w:tcPr><w:tcW w:type="dxa" w:w="%d"/><w:tcMar>`, widths[j])
			for _, side := range []string{"top", "left", "bottom", "right"} {
				fmt.Fprintf(&t, `<w:%s w:w="%d" w:type="dxa"/>`, side, margin)
			}
			t.WriteString("</w:tcMar></w:tcPr>")
			a := ""
			if i < len(blk.CellAlign) && j < len(blk.CellAlign[i]) {
				a = alignMap[blk.CellAlign[i][j]]
			}
			t.WriteString(para("", a, "", runXML(Run{Text: cell, Bold: i == 0 && header && headerBold})))
			t.WriteString("</w:tc>")
		}
		t.WriteString("</w:tr>")
	}
	t.WriteString("</w:tbl>")
	b.body.WriteString(t.String())
	return nil
}

func (b *builder) image(blk Block, align string) error {
	p := blk.Path
	if !filepath.IsAbs(p) && b.base != "" {
		p = filepath.Join(b.base, p)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("image %s: %w", blk.Path, err)
	}
	if format == "jpeg" {
		format = "jpg"
	}
	width := blk.WidthIn
	if width <= 0 {
		width = min(6, float64(b.usableTwips())/1440)
	}
	cx := int64(width * 914400)
	cy := cx * int64(cfg.Height) / int64(max(cfg.Width, 1))
	b.imgN++
	name := fmt.Sprintf("media/image%d.%s", b.imgN, format)
	b.pkg.Set("word/"+name, raw)
	b.pkg.EnsureContentType("."+format, map[string]string{"png": "image/png", "jpg": "image/jpeg", "gif": "image/gif"}[format])
	id := b.rel("image", name, false)
	descr := esc(blk.Description)
	drawing := fmt.Sprintf(`<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="Picture %d" descr="%s"/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="%d" name="Picture %d" descr="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`,
		cx, cy, b.imgN, b.imgN, descr, b.imgN, b.imgN, descr, id, cx, cy)
	b.body.WriteString(para("", align, "", drawing))
	return nil
}

// Create writes a new DOCX described by spec; baseDir resolves image paths.
func Create(spec Spec, baseDir, output string) error {
	if spec.Page.WidthIn == 0 {
		spec.Page.WidthIn, spec.Page.HeightIn = 8.5, 11
	}
	if spec.Page.MarginIn == 0 {
		spec.Page.MarginIn = 1
	}
	if spec.Font == "" {
		spec.Font = "Calibri"
	}
	if spec.FontSize == 0 {
		spec.FontSize = 11
	}
	data, err := Build(spec, baseDir)
	if err != nil {
		return err
	}
	return opc.WriteNew(output, data)
}

// Build returns DOCX bytes for spec.
func Build(spec Spec, baseDir string) ([]byte, error) {
	return build(spec, baseDir, time.Now)
}

func build(spec Spec, baseDir string, clock func() time.Time) ([]byte, error) {
	if spec.StylesFile != "" && spec.StylesOverlayFile != "" {
		return nil, errors.New("styles_file and styles_overlay_file are mutually exclusive")
	}
	pkg, err := docxtemplate.Open()
	if err != nil {
		return nil, err
	}
	// Keep the pinned template settings, including proofing and compatibility
	// controls. Only the explicitly supported compatibility mode is changed.
	switch spec.CompatibilityMode {
	case 0, 15:
		pkg.Set("word/settings.xml", bytes.Replace(pkg.Get("word/settings.xml"),
			[]byte(`w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="14"`),
			[]byte(`w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"`), 1))
	case 14:
	default:
		return nil, errors.New("compatibility_mode must be 14 or 15")
	}
	rels, err := pkg.XML("word/_rels/document.xml.rels")
	if err != nil {
		return nil, err
	}
	b := &builder{spec: spec, pkg: pkg, base: baseDir}
	for _, rel := range rels.Root().ChildElements() {
		n, err := strconv.Atoi(strings.TrimPrefix(rel.SelectAttrValue("Id", ""), "rId"))
		if err != nil {
			return nil, fmt.Errorf("invalid template relationship ID: %w", err)
		}
		b.relN = max(b.relN, n)
	}
	for i, blk := range spec.Blocks {
		if err := b.block(blk); err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
	}
	// Keep template numbering unless authored lists require our explicit definitions.
	if b.numbered || b.bulleted {
		pkg.Set("word/numbering.xml", []byte(xmlHead+numberingXMLForOrderedLists(b.orderedListCount)))
	}
	// Retain the template's namespace context and section provenance. Replace
	// authored blocks and section properties, not the document shell.
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		return nil, err
	}
	doc.WriteSettings.CanonicalText = true
	body := doc.Root().FindElement("w:body")
	section := body.FindElement("w:sectPr")
	generated := etree.NewDocument()
	if err := generated.ReadFromString("<body>" + b.body.String() + b.sectPr() + "</body>"); err != nil {
		return nil, err
	}
	generated.Root().FindElement("w:sectPr").Attr = append([]etree.Attr(nil), section.Attr...)
	for _, child := range append([]etree.Token(nil), body.Child...) {
		body.RemoveChild(child)
	}
	for _, child := range generated.Root().ChildElements() {
		body.AddChild(child.Copy())
	}
	if err := pkg.SetXML("word/document.xml", doc); err != nil {
		return nil, err
	}

	styles := []byte(stylesXML(spec.Font, spec.FontSize))
	if spec.StylesFile != "" {
		styles, err = readXMLResource(baseDir, spec.StylesFile, NSW, "styles")
		if err != nil {
			return nil, err
		}
	}
	if spec.StylesOverlayFile != "" {
		overlay, err := readXMLResource(baseDir, spec.StylesOverlayFile, NSW, "styles")
		if err != nil {
			return nil, err
		}
		styles, err = overlayStyles(pkg.Get("word/styles.xml"), overlay)
		if err != nil {
			return nil, err
		}
	}
	pkg.Set("word/styles.xml", styles)
	if spec.ThemeFile != "" {
		theme, err := readXMLResource(baseDir, spec.ThemeFile, "http://schemas.openxmlformats.org/drawingml/2006/main", "theme")
		if err != nil {
			return nil, err
		}
		pkg.Set("word/theme/theme1.xml", theme)
		if err := pkg.EnsureContentType("word/theme/theme1.xml", "application/vnd.openxmlformats-officedocument.theme+xml"); err != nil {
			return nil, err
		}
	}
	// Keep original relationship bytes when no new resources are added.
	if len(b.docRels) > 0 {
		for _, raw := range b.docRels {
			d := etree.NewDocument()
			if err := d.ReadFromString(raw); err != nil {
				return nil, err
			}
			rels.Root().AddChild(d.Root())
		}
		if err := pkg.SetXML("word/_rels/document.xml.rels", rels); err != nil {
			return nil, err
		}
	}
	// Preserve reference template metadata unless the caller supplies a field.
	if spec.Title != "" || spec.Author != "" {
		d, err := pkg.XML("docProps/core.xml")
		if err != nil {
			return nil, err
		}
		for _, field := range []struct{ name, value string }{{"dc:title", spec.Title}, {"dc:creator", spec.Author}} {
			if field.value == "" {
				continue
			}
			e := d.Root().FindElement(field.name)
			if e == nil {
				e = d.Root().CreateElement(field.name)
			}
			e.SetText(field.value)
		}
		if err := pkg.SetXML("docProps/core.xml", d); err != nil {
			return nil, err
		}
	}
	return pkg.DocumentBytes(clock)
}

func (b *builder) sectPr() string {
	pg := b.spec.Page
	w, h := int(pg.WidthIn*1440), int(pg.HeightIn*1440)
	orient := ""
	if pg.Landscape {
		w, h = h, w
		orient = ` w:orient="landscape"`
	}
	m := int(pg.MarginIn * 1440)
	var refs strings.Builder
	if b.spec.Header != "" {
		id := b.rel("header", "header1.xml", false)
		b.pkg.Set("word/header1.xml", []byte(hfXML("hdr", para("Header", "", "", runXML(Run{Text: b.spec.Header})))))
		b.pkg.EnsureContentType("/word/header1.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml")
		fmt.Fprintf(&refs, `<w:headerReference w:type="default" r:id="%s"/>`, id)
	}
	if b.spec.Footer != "" || b.spec.PageNumbers {
		content := ""
		if b.spec.Footer != "" {
			content += runXML(Run{Text: b.spec.Footer})
		}
		if b.spec.PageNumbers {
			if content != "" {
				content += runXML(Run{Text: "\t"})
			}
			content += `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>1</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`
		}
		id := b.rel("footer", "footer1.xml", false)
		b.pkg.Set("word/footer1.xml", []byte(hfXML("ftr", para("Footer", "", "", content))))
		b.pkg.EnsureContentType("/word/footer1.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml")
		fmt.Fprintf(&refs, `<w:footerReference w:type="default" r:id="%s"/>`, id)
	}
	return fmt.Sprintf(`<w:sectPr>%s<w:pgSz w:w="%d" w:h="%d"%s/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="720" w:footer="720" w:gutter="0"/><w:cols w:space="720"/><w:docGrid w:linePitch="360"/></w:sectPr>`, refs.String(), w, h, orient, m, m, m, m)
}

func hfXML(tag, content string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<w:` + tag + ` xmlns:w="` + NSW + `" xmlns:r="` + opc.NSOffR + `">` + content + `</w:` + tag + `>`
}

// readXMLResource imports an explicit authoring resource without rewriting its
// inherited style or theme properties. It does not import external relationships.
func readXMLResource(base, name, namespace, local string) ([]byte, error) {
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	root := doc.Root()
	if root == nil || root.Tag != local || root.NamespaceURI() != namespace {
		return nil, fmt.Errorf("%s: expected {%s}%s root", name, namespace, local)
	}
	return raw, nil
}
