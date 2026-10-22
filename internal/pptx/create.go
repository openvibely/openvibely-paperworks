// Package pptx authors native, editable PresentationML packages.
package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

const pns = "http://schemas.openxmlformats.org/presentationml/2006/main"
const ans = "http://schemas.openxmlformats.org/drawingml/2006/main"
const rns = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
const cns = "http://schemas.openxmlformats.org/drawingml/2006/chart"
const ns = ` xmlns:p="` + pns + `" xmlns:a="` + ans + `" xmlns:r="` + rns + `"`
const group = `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`
const colors = `<p:clrMap accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" bg1="lt1" bg2="lt2" folHlink="folHlink" hlink="hlink" tx1="dk1" tx2="dk2"/>`
const override = `<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>`

type Spec struct {
	StyleResources *StyleResources `json:"style_resources,omitempty"`
	Width          float64         `json:"width_in"`
	Height         float64         `json:"height_in"`
	Font           string          `json:"font"`
	Title          string          `json:"title"`
	Slides         []Slide         `json:"slides"`
}
type Slide struct {
	NotesParagraphs []Paragraph `json:"notes_paragraphs,omitempty"`
	Layout          *Layout     `json:"layout,omitempty"`
	Background      string      `json:"background"`
	Notes           string      `json:"notes"`
	Elements        []Element   `json:"elements"`
}
type Run struct {
	Text      string  `json:"text"`
	FontSize  float64 `json:"font_size"`
	Color     string  `json:"color"`
	Bold      *bool   `json:"bold"`
	Italic    bool    `json:"italic"`
	Underline bool    `json:"underline"`
}
type Paragraph struct {
	Format *ParagraphStyle `json:"format,omitempty"`
	Runs   []Run           `json:"runs"`
	Text   string          `json:"text"`
	Bullet bool            `json:"bullet"`
	Level  int             `json:"level"`
	Bold   bool            `json:"bold"`
}
type Series struct {
	Sizes  []float64 `json:"sizes,omitempty"`
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}
type Chart struct {
	Options           *excelize.Chart `json:"options,omitempty"`
	InheritFormatting bool            `json:"inherit_formatting,omitempty"`
	Type              string          `json:"type"`
	Title             string          `json:"title"`
	Categories        []string        `json:"categories"`
	Series            []Series        `json:"series"`
}

// Crop fractions describe source pixels removed from each edge (0..1).
type Crop struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

// ImageAnchor positions the visible region: 0 is left/top, 1 is right/bottom.
type ImageAnchor struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Element struct {
	DrawingStyle      *DrawingStyle `json:"drawing_style,omitempty"`
	Connector         *Connector    `json:"connector,omitempty"`
	Commands          []PathCommand `json:"commands,omitempty"`
	InheritFormatting bool          `json:"inherit_formatting,omitempty"`
	Placeholder       string        `json:"placeholder"`
	Anchor            *ImageAnchor  `json:"anchor"`
	Fit               string        `json:"fit"`
	Crop              *Crop         `json:"crop"`
	Type              string        `json:"type"`
	Name              string        `json:"name"`
	X                 float64       `json:"x"`
	Y                 float64       `json:"y"`
	W                 float64       `json:"w"`
	H                 float64       `json:"h"`
	Text              string        `json:"text"`
	Paragraphs        []Paragraph   `json:"paragraphs"`
	FontSize          float64       `json:"font_size"`
	Bold              bool          `json:"bold"`
	Color             string        `json:"color"`
	Fill              string        `json:"fill"`
	Align             string        `json:"align"`
	Shape             string        `json:"shape"`
	Rows              [][]string    `json:"rows"`
	Path              string        `json:"path"`
	Description       string        `json:"description"`
	Chart             *Chart        `json:"chart"`
}

func esc(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }

// Match reference presentation library Inches: multiply the floating-point value, then truncate.
// Rounding changes serialized slide and shape dimensions by one EMU.
func emu(v float64) int64   { return int64(v * 914400) }
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

var hex = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

func color(v, def string) (string, error) {
	if v == "" {
		return def, nil
	}
	if !hex.MatchString(v) {
		return "", fmt.Errorf("color %q must be six hex digits", v)
	}
	return v, nil
}
func rel(id, kind, target string) string {
	return `<Relationship Id="` + id + `" Type="` + rns + `/` + kind + `" Target="` + esc(target) + `"/>`
}
func relationships(s string) []byte {
	return []byte(packageXMLDeclaration + `<Relationships xmlns="` + opc.NSRel + `">` + s + `</Relationships>`)
}
func xfrm(e Element, prefix string) string {
	return fmt.Sprintf(`<%s:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></%s:xfrm>`, prefix, emu(e.X), emu(e.Y), emu(e.W), emu(e.H), prefix)
}
func nv(id int, e Element, kind string) string {
	properties := `<p:nvPr/>`
	if e.Placeholder != "" {
		properties = `<p:nvPr><p:ph type="` + esc(e.Placeholder) + `"/></p:nvPr>`
	}
	return fmt.Sprintf(`<p:nv%sPr><p:cNvPr id="%d" name="%s" descr="%s"/><p:cNv%sPr/>%s</p:nv%sPr>`, kind, id, esc(e.Name), esc(e.Description), kind, properties, kind)
}
func textBody(e Element, font string) string {
	return authoredTextBody(e, font, false)
}

// forEachTextParagraph applies the shared legacy text and paragraph-run
// fallbacks while leaving each text body's formatting policy to its caller.
func forEachTextParagraph(e Element, visit func(Paragraph, []Run)) {
	paragraphs := e.Paragraphs
	if len(paragraphs) == 0 {
		paragraphs = make([]Paragraph, 0, strings.Count(e.Text, "\n")+1)
		for _, line := range strings.Split(e.Text, "\n") {
			paragraphs = append(paragraphs, Paragraph{Text: line})
		}
	}
	for _, paragraph := range paragraphs {
		runs := paragraph.Runs
		if len(runs) == 0 {
			runs = []Run{{Text: paragraph.Text}}
		}
		visit(paragraph, runs)
	}
}

func authoredTextBody(e Element, font string, inheritBold bool) string {
	if e.InheritFormatting {
		return inheritedTextBody(e)
	}
	sz := e.FontSize
	if sz == 0 {
		sz = 18
	}
	col := e.Color
	if col == "" {
		col = "172B4D"
	}
	align := e.Align
	if align == "" {
		align = "l"
	}
	var b strings.Builder
	b.WriteString(`<a:bodyPr wrap="square" lIns="91440" rIns="91440" tIns="45720" bIns="45720"/><a:lstStyle/>`)
	forEachTextParagraph(e, func(p Paragraph, runs []Run) {
		b.WriteString(`<a:p>`)
		if p.Bullet {
			fmt.Fprintf(&b, `<a:pPr algn="%s" lvl="%d" marL="%d" indent="-228600"><a:buChar char="•"/></a:pPr>`, align, p.Level, 342900+int64(p.Level)*228600)
		} else {
			fmt.Fprintf(&b, `<a:pPr algn="%s"><a:buNone/></a:pPr>`, align)
		}
		for _, r := range runs {
			bold, italic := 0, 0
			if p.Bold || e.Bold {
				bold = 1
			}
			if r.Bold != nil {
				bold = 0
				if *r.Bold {
					bold = 1
				}
			}
			if r.Italic {
				italic = 1
			}
			underline := "none"
			if r.Underline {
				underline = "sng"
			}
			size, ink := sz, col
			if r.FontSize != 0 {
				size = r.FontSize
			}
			if r.Color != "" {
				ink = r.Color
			}
			boldAttr := fmt.Sprintf(` b="%d"`, bold)
			// An absent table-run setting inherits the table style. Explicit
			// run false must still override a bold table style.
			if inheritBold && !p.Bold && !e.Bold && r.Bold == nil {
				boldAttr = ""
			}
			decoration := fmt.Sprintf(` i="%d" u="%s"`, italic, underline)
			if inheritBold {
				decoration = ""
				if r.Italic {
					decoration += ` i="1"`
				}
				if r.Underline {
					decoration += ` u="sng"`
				}
			}
			fmt.Fprintf(&b, `<a:r><a:rPr lang="en-US" sz="%d"%s%s><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/></a:rPr><a:t>%s</a:t></a:r>`, int(size*100), boldAttr, decoration, ink, esc(font), esc(r.Text))
		}
		fmt.Fprintf(&b, `<a:endParaRPr lang="en-US" sz="%d"/></a:p>`, int(sz*100))
	})
	return b.String()
}

type builder struct {
	clock          func() time.Time
	p              *opc.Package
	ct             strings.Builder
	images, charts int
}

func (b *builder) add(name, contentType, body string) {
	b.p.Set(name, []byte(body))
	if contentType != "" {
		fmt.Fprintf(&b.ct, `<Override PartName="/%s" ContentType="%s"/>`, name, contentType)
	}
}
func (b *builder) content(name, kind, body string) {
	b.add(name, "application/vnd.openxmlformats-officedocument.presentationml."+kind+"+xml", body)
}
func emptyPackage() *opc.Package {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	z.Close()
	p, _ := opc.Read(buf.Bytes())
	return p
}

func Create(ctx context.Context, s Spec, base, output string) error {
	return createWithClock(ctx, s, base, output, time.Now)
}
func createWithClock(ctx context.Context, s Spec, base, output string, clock func() time.Time) error {
	if len(s.Slides) < 1 || len(s.Slides) > 1000 {
		return fmt.Errorf("slides must contain 1..1000 entries")
	}
	if s.Width == 0 {
		s.Width = 13.333333
	}
	if s.Height == 0 {
		s.Height = 7.5
	}
	if s.Font == "" {
		s.Font = "Arial"
	}
	if !finite(s.Width) || !finite(s.Height) || s.Width < 1 || s.Height < 1 || s.Width > 56 || s.Height > 56 {
		return fmt.Errorf("slide size must be 1..56 inches")
	}
	if s.StyleResources != nil && s.StyleResources.TemplateLayout != nil {
		index := *s.StyleResources.TemplateLayout
		if index < 0 || index > 10 {
			return fmt.Errorf("template_layout must be 0..10")
		}
		if s.StyleResources.Master != "" || s.StyleResources.Layout != "" {
			return fmt.Errorf("template_layout cannot be combined with custom master or layout")
		}
	}
	b := &builder{p: emptyPackage(), clock: clock}
	b.ct.WriteString(`<Types xmlns="` + opc.NSCT + `"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="jpg" ContentType="image/jpeg"/><Default Extension="xlsx" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"/>`)
	if err := b.addTemplateResources(); err != nil {
		return err
	}
	b.p.Set("_rels/.rels", relationships(rel("rId1", "officeDocument", "ppt/presentation.xml")+`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/thumbnail" Target="docProps/thumbnail.jpeg"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>`+rel("rId4", "extended-properties", "docProps/app.xml")))
	b.add("docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml", `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>`+esc(s.Title)+`</dc:title><dc:creator>Paperworks</dc:creator></cp:coreProperties>`)
	b.add("ppt/theme/theme1.xml", "application/vnd.openxmlformats-officedocument.theme+xml", theme(s.Font))
	slideLayout := "../slideLayouts/slideLayout1.xml"
	if s.StyleResources != nil && s.StyleResources.TemplateLayout != nil {
		if err := b.addTemplateLayouts(); err != nil {
			return err
		}
		slideLayout = fmt.Sprintf("../slideLayouts/slideLayout%d.xml", *s.StyleResources.TemplateLayout+1)
	} else {
		b.content("ppt/slideMasters/slideMaster1.xml", "slideMaster", `<p:sldMaster`+ns+`><p:cSld><p:spTree>`+group+`</p:spTree></p:cSld>`+colors+`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst><p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles></p:sldMaster>`)
		b.p.Set("ppt/slideMasters/_rels/slideMaster1.xml.rels", relationships(rel("rId1", "slideLayout", slideLayout)+rel("rId2", "theme", "../theme/theme1.xml")))
		b.content("ppt/slideLayouts/slideLayout1.xml", "slideLayout", `<p:sldLayout`+ns+` type="blank" preserve="1"><p:cSld name="Blank"><p:spTree>`+group+`</p:spTree></p:cSld>`+override+`</p:sldLayout>`)
		b.p.Set("ppt/slideLayouts/_rels/slideLayout1.xml.rels", relationships(rel("rId1", "slideMaster", "../slideMasters/slideMaster1.xml")))
		b.content("ppt/notesMasters/notesMaster1.xml", "notesMaster", `<p:notesMaster`+ns+`><p:cSld><p:spTree>`+group+noteShape("", s.Font)+`</p:spTree></p:cSld>`+colors+`<p:notesStyle/></p:notesMaster>`)
		b.p.Set("ppt/notesMasters/_rels/notesMaster1.xml.rels", relationships(rel("rId1", "theme", "../theme/theme1.xml")))
	}
	defaultText := "<p:defaultTextStyle/>"
	if s.StyleResources != nil {
		for _, r := range []struct{ name, part, ns, tag string }{
			{s.StyleResources.Theme, "ppt/theme/theme1.xml", ans, "theme"},
			{s.StyleResources.Master, "ppt/slideMasters/slideMaster1.xml", pns, "sldMaster"},
			{s.StyleResources.Layout, "ppt/slideLayouts/slideLayout1.xml", pns, "sldLayout"},
			{s.StyleResources.DefaultText, "", pns, "defaultTextStyle"},
		} {
			if r.name == "" {
				continue
			}
			data, err := readStyleResource(base, r.name, r.ns, r.tag)
			if err != nil {
				return err
			}
			if r.part == "" {
				defaultText = string(data)
			} else {
				b.p.Set(r.part, data)
			}
		}
	}
	hasTableStyles := false
	if s.StyleResources != nil && s.StyleResources.TableStyles != "" {
		raw, err := readStyleResource(base, s.StyleResources.TableStyles, ans, "tblStyleLst")
		if err != nil {
			return err
		}
		b.add("ppt/tableStyles.xml", "application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml", string(raw))
		hasTableStyles = true
	}
	var slideIDs strings.Builder
	presRels := relationshipList{}
	presRels.add("slideMaster", "slideMasters/slideMaster1.xml")
	presRels.add("printerSettings", "printerSettings/printerSettings1.bin")
	presRels.add("presProps", "presProps.xml")
	presRels.add("viewProps", "viewProps.xml")
	presRels.add("theme", "theme/theme1.xml")
	if hasTableStyles {
		presRels.add("tableStyles", "tableStyles.xml")
	}
	notesMasterID := ""
	count := 0
	for i, sl := range s.Slides {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := i + 1
		slideID := presRels.add("slide", fmt.Sprintf("slides/slide%d.xml", n))
		if notesMasterID == "" {
			notesMasterID = presRels.add("notesMaster", "notesMasters/notesMaster1.xml")
		}
		bg, err := color(sl.Background, "FFFFFF")
		if err != nil {
			return err
		}
		var shapes strings.Builder
		rels := relationshipList{}
		rels.add("slideLayout", slideLayout)
		for j, e := range sl.Elements {
			count++
			if count > 10000 {
				return fmt.Errorf("element limit exceeded")
			}
			if !finite(e.X) || !finite(e.Y) || !finite(e.W) || !finite(e.H) || e.X < 0 || e.Y < 0 || e.W <= 0 || e.H <= 0 || e.X+e.W > s.Width+0.00001 || e.Y+e.H > s.Height+0.00001 {
				return fmt.Errorf("slide %d element %d: invalid or off-slide geometry", n, j+1)
			}
			if e.Placeholder != "" && (e.Type != "text" || (e.Placeholder != "title" && e.Placeholder != "ctrTitle")) {
				return fmt.Errorf("placeholder must be title or ctrTitle on a text element")
			}
			if e.Type != "image" && (e.Fit != "" || e.Crop != nil || e.Anchor != nil) {
				return fmt.Errorf("fit, crop and anchor apply only to images")
			}
			if _, err = color(e.Color, ""); err != nil {
				return err
			}
			if _, err = color(e.Fill, ""); err != nil {
				return err
			}
			if !finite(e.FontSize) || e.FontSize < 0 || e.FontSize > 400 {
				return fmt.Errorf("invalid font size")
			}
			if e.Align != "" && e.Align != "l" && e.Align != "ctr" && e.Align != "r" && e.Align != "just" {
				return fmt.Errorf("invalid alignment")
			}
			for _, p := range e.Paragraphs {
				if len(p.Runs) > 0 && p.Text != "" {
					return fmt.Errorf("paragraph text and runs are mutually exclusive")
				}
				if len(p.Runs) > 10000 {
					return fmt.Errorf("paragraph run limit exceeded")
				}
				for _, r := range p.Runs {
					if !finite(r.FontSize) || r.FontSize < 0 || r.FontSize > 400 {
						return fmt.Errorf("invalid run font size")
					}
					if _, err = color(r.Color, ""); err != nil {
						return err
					}
				}
				if p.Level < 0 || p.Level > 8 {
					return fmt.Errorf("list level must be 0..8")
				}
			}
			if e.Name == "" {
				e.Name = fmt.Sprintf("%s %d", e.Type, j+1)
			}
			id := j + 2
			switch e.Type {
			case "connector", "path":
				native, err := drawingElement(id, e)
				if err != nil {
					return err
				}
				shapes.WriteString(native)
			case "text", "shape":
				geom := e.Shape
				if geom == "" {
					geom = "rect"
				}
				switch geom {
				case "rect", "roundRect", "ellipse", "triangle", "chevron", "rightArrow", "leftArrow", "upArrow", "downArrow", "leftRightArrow", "upDownArrow", "pentagon", "hexagon", "heptagon", "octagon", "diamond", "parallelogram", "trapezoid", "star4", "star5", "star6", "star8", "star12", "star16", "star24", "star32", "heart", "cloud", "arc", "pie", "donut", "blockArc", "bracePair", "bracketPair", "plus", "mathMinus", "mathMultiply", "mathDivide", "flowChartProcess", "flowChartDecision", "flowChartTerminator", "flowChartDocument", "wedgeRectCallout", "wedgeRoundRectCallout", "wedgeEllipseCallout":
				default:
					return fmt.Errorf("unsupported shape %q", geom)
				}
				fill := `<a:noFill/>`
				if e.Fill != "" {
					fill = `<a:solidFill><a:srgbClr val="` + e.Fill + `"/></a:solidFill>`
				}
				shapes.WriteString(`<p:sp>` + nv(id, e, "Sp") + `<p:spPr>` + xfrm(e, "a") + `<a:prstGeom prst="` + geom + `"><a:avLst/></a:prstGeom>` + fill + `<a:ln><a:noFill/></a:ln></p:spPr><p:txBody>` + textBody(e, s.Font) + `</p:txBody></p:sp>`)
			case "table":
				if len(e.Rows) == 0 || len(e.Rows) > 1000 || len(e.Rows[0]) == 0 || len(e.Rows[0]) > 100 {
					return fmt.Errorf("table requires 1..1000 rows and 1..100 columns")
				}
				var table strings.Builder
				table.WriteString(`<a:tbl><a:tblPr firstRow="1" bandRow="1"><a:tableStyleId>{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}</a:tableStyleId></a:tblPr><a:tblGrid>`)
				cols := len(e.Rows[0])
				for k := 0; k < cols; k++ {
					width := emu(e.W) / int64(cols)
					if k == cols-1 {
						width = emu(e.W) - int64(cols-1)*width
					}
					fmt.Fprintf(&table, `<a:gridCol w="%d"/>`, width)
				}
				table.WriteString(`</a:tblGrid>`)
				for ri, row := range e.Rows {
					if len(row) != cols {
						return fmt.Errorf("table rows must have equal lengths")
					}
					height := emu(e.H) / int64(len(e.Rows))
					if ri == len(e.Rows)-1 {
						height = emu(e.H) - int64(len(e.Rows)-1)*height
					}
					fmt.Fprintf(&table, `<a:tr h="%d">`, height)
					for _, value := range row {
						cell := e
						cell.Text = value
						cell.Paragraphs = nil
						// Leave unspecified cell fills to the table style, as
						// reference presentation library does. Explicit element formatting still applies.
						fill := ""
						if e.Fill != "" {
							fill = `<a:solidFill><a:srgbClr val="` + e.Fill + `"/></a:solidFill>`
						}
						table.WriteString(`<a:tc><a:txBody>` + authoredTextBody(cell, s.Font, true) + `</a:txBody><a:tcPr>` + fill + `</a:tcPr></a:tc>`)
					}
					table.WriteString(`</a:tr>`)
				}
				table.WriteString(`</a:tbl>`)
				shapes.WriteString(frame(id, e, "http://schemas.openxmlformats.org/drawingml/2006/table", table.String()))
			case "image":
				path := e.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(base, path)
				}
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				info, err := f.Stat()
				if err != nil {
					f.Close()
					return err
				}
				if !info.Mode().IsRegular() || info.Size() > 32<<20 {
					f.Close()
					return fmt.Errorf("image must be a regular file <=32 MiB")
				}
				cfg, format, err := image.DecodeConfig(f)
				f.Close()
				if err != nil {
					return err
				}
				ext := "png"
				if format == "jpeg" {
					ext = "jpg"
				} else if format != "png" {
					return fmt.Errorf("only PNG/JPEG images are supported")
				}
				if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 100000000 {
					return fmt.Errorf("image dimensions exceed limits")
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				b.images++
				name := fmt.Sprintf("image%d.%s", b.images, ext)
				b.p.Set("ppt/media/"+name, data)
				rid := rels.add("image", "../media/"+name)
				e, crop, err := imagePlacement(e, cfg.Width, cfg.Height)
				if err != nil {
					return err
				}
				shapes.WriteString(`<p:pic>` + nv(id, e, "Pic") + `<p:blipFill><a:blip r:embed="` + rid + `"/>` + crop + `<a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr>` + xfrm(e, "a") + `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`)

			case "chart":
				if e.Chart == nil {
					return fmt.Errorf("chart data required")
				}
				part, err := b.chart(ctx, *e.Chart, s.Font)
				if err != nil {
					return err
				}
				rid := rels.add("chart", "../charts/"+part)
				shapes.WriteString(frame(id, e, cns, `<c:chart xmlns:c="`+cns+`" r:id="`+rid+`"/>`))
			default:
				return fmt.Errorf("unsupported element type %q", e.Type)
			}
		}
		name := fmt.Sprintf("ppt/slides/slide%d.xml", n)
		b.content(name, "slide", `<p:sld`+ns+`><p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="`+bg+`"/></a:solidFill><a:effectLst/></p:bgPr></p:bg><p:spTree>`+group+shapes.String()+`</p:spTree></p:cSld>`+override+`</p:sld>`)
		rels.add("notesSlide", fmt.Sprintf("../notesSlides/notesSlide%d.xml", n))
		b.p.Set(opc.RelsName(name), relationships(rels.String()))
		notes := fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", n)
		b.content(notes, "notesSlide", `<p:notes`+ns+`><p:cSld><p:spTree>`+group+noteShape(sl.Notes, s.Font)+`</p:spTree></p:cSld>`+override+`</p:notes>`)
		b.p.Set(opc.RelsName(notes), relationships(rel("rId1", "notesMaster", "../notesMasters/notesMaster1.xml")+rel("rId2", "slide", fmt.Sprintf("../slides/slide%d.xml", n))))
		fmt.Fprintf(&slideIDs, `<p:sldId id="%d" r:id="%s"/>`, 256+i, slideID)
	}
	if notesMasterID == "" {
		notesMasterID = presRels.add("notesMaster", "notesMasters/notesMaster1.xml")
	}
	b.content("ppt/presentation.xml", "presentation.main", fmt.Sprintf(`<p:presentation%s><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst><p:notesMasterIdLst><p:notesMasterId r:id="%s"/></p:notesMasterIdLst><p:sldIdLst>%s</p:sldIdLst><p:sldSz cx="%d" cy="%d"/><p:notesSz cx="6858000" cy="9144000"/>%s</p:presentation>`, ns, notesMasterID, slideIDs.String(), emu(s.Width), emu(s.Height), defaultText))
	b.p.Set("ppt/_rels/presentation.xml.rels", relationships(presRels.String()))
	b.ct.WriteString(`</Types>`)
	manifest, err := b.contentTypes()
	if err != nil {
		return err
	}
	b.p.Set("[Content_Types].xml", manifest)
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.StyleResources != nil && s.StyleResources.TemplateLayout != nil {
		if err := b.finalizeTemplateContent(s); err != nil {
			return err
		}
	}
	for i, slide := range s.Slides {
		name := fmt.Sprintf("ppt/slides/slide%d.xml", i+1)
		changed := false
		d, err := b.p.XML(name)
		if err != nil {
			return err
		}
		shapes := office.Child(office.Child(d.Root(), pns, "cSld"), pns, "spTree").ChildElements()[2:]
		// Measure final paragraph formatting before sizing and positioning shapes.
		for j, e := range slide.Elements {
			if body := office.Child(shapes[j], pns, "txBody"); body != nil {
				paragraphs := office.Children(body, ans, "p")
				for k, paragraph := range e.Paragraphs {
					if paragraph.Format != nil {
						if k >= len(paragraphs) {
							return fmt.Errorf("paragraph format index out of range")
						}
						if err := styleParagraph(paragraphs[k], paragraph.Format); err != nil {
							return err
						}
						changed = true
					}
				}
			}
		}
		if slide.Layout != nil {
			if err := layoutObjects(b.p, name, d.Root(), slide.Layout); err != nil {
				return err
			}
			changed = true
		}
		ids := map[string]bool{}
		for _, prop := range office.All(d.Root(), pns, "cNvPr") {
			ids[prop.SelectAttrValue("id", "")] = true
		}
		for j, e := range slide.Elements {
			if e.Connector != nil {
				for _, conn := range []*Connection{e.Connector.Start, e.Connector.End} {
					if conn != nil && !ids[conn.ShapeID] {
						return fmt.Errorf("connector target %s not found", conn.ShapeID)
					}
				}
			}

			if e.DrawingStyle != nil {
				if err := styleDrawingInPackage(b.p, name, shapes[j], e.DrawingStyle, base); err != nil {
					return err
				}
				changed = true
			}
		}
		if slide.NotesParagraphs != nil {
			if slide.Notes != "" {
				return fmt.Errorf("notes and notes_paragraphs are mutually exclusive")
			}
			notesPart := fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i+1)
			nd, err := b.p.XML(notesPart)
			if err != nil {
				return err
			}
			bodies := office.All(nd.Root(), pns, "txBody")
			if len(bodies) != 1 {
				return fmt.Errorf("notes body is ambiguous")
			}
			if err := replaceRichNotes(bodies[0], slide.NotesParagraphs, s.Font); err != nil {
				return err
			}
			if err := b.p.SetXML(notesPart, nd); err != nil {
				return err
			}
		}
		if changed {
			if err := b.p.SetXML(name, d); err != nil {
				return err
			}
		}
	}
	if s.StyleResources != nil && s.StyleResources.TemplateLayout != nil {
		raw, err := presentationBytes(b.p, clock)
		if err != nil {
			return err
		}
		return opc.WriteNew(output, raw)
	}
	return b.p.SaveNew(output)
}
func frame(id int, e Element, uri, body string) string {
	return `<p:graphicFrame>` + nv(id, e, "GraphicFrame") + xfrm(e, "p") + `<a:graphic><a:graphicData uri="` + uri + `">` + body + `</a:graphicData></a:graphic></p:graphicFrame>`
}
func noteShape(text, font string) string {
	return `<p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes"/><p:cNvSpPr txBox="1"/><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody>` + textBody(Element{Text: text, FontSize: 12}, font) + `</p:txBody></p:sp>`
}
func theme(font string) string {
	var c strings.Builder
	for _, pair := range [][2]string{{"dk1", "000000"}, {"lt1", "FFFFFF"}, {"dk2", "172B4D"}, {"lt2", "F2F5F9"}, {"accent1", "2463EB"}, {"accent2", "10A58E"}, {"accent3", "F5A623"}, {"accent4", "805AD5"}, {"accent5", "D64550"}, {"accent6", "527A91"}, {"hlink", "0563C1"}, {"folHlink", "954F72"}} {
		fmt.Fprintf(&c, `<a:%s><a:srgbClr val="%s"/></a:%s>`, pair[0], pair[1], pair[0])
	}
	fill := `<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`
	line := `<a:ln w="9525">` + fill + `<a:prstDash val="solid"/></a:ln>`
	return `<a:theme xmlns:a="` + ans + `" name="Paperworks"><a:themeElements><a:clrScheme name="Paperworks">` + c.String() + `</a:clrScheme><a:fontScheme name="Paperworks"><a:majorFont><a:latin typeface="` + esc(font) + `"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont><a:minorFont><a:latin typeface="` + esc(font) + `"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont></a:fontScheme><a:fmtScheme name="Paperworks"><a:fillStyleLst>` + strings.Repeat(fill, 3) + `</a:fillStyleLst><a:lnStyleLst>` + strings.Repeat(line, 3) + `</a:lnStyleLst><a:effectStyleLst>` + strings.Repeat(`<a:effectStyle><a:effectLst/></a:effectStyle>`, 3) + `</a:effectStyleLst><a:bgFillStyleLst>` + strings.Repeat(fill, 3) + `</a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`
}
func (b *builder) chart(ctx context.Context, c Chart, font string) (string, error) {
	types := map[string]excelize.ChartType{"column": excelize.Col, "bar": excelize.Bar, "line": excelize.Line, "pie": excelize.Pie, "doughnut": excelize.Doughnut, "area": excelize.Area, "scatter": excelize.Scatter, "radar": excelize.Radar, "bubble": excelize.Bubble, "area3D": excelize.Area3D, "bar3D": excelize.Bar3DClustered, "pie3D": excelize.Pie3D, "line3D": excelize.Line3D, "stock-hlc": excelize.StockHighLowClose, "stock-ohlc": excelize.StockOpenHighLowClose}
	typ, ok := types[c.Type]
	if !ok {
		return "", fmt.Errorf("unsupported chart type %q", c.Type)
	}
	if len(c.Categories) < 1 || len(c.Categories) > 10000 || len(c.Series) < 1 || len(c.Series) > 100 {
		return "", fmt.Errorf("chart requires 1..10000 categories and 1..100 series")
	}
	if (c.Type == "pie" || c.Type == "doughnut") && len(c.Series) != 1 {
		return "", fmt.Errorf("pie/doughnut charts require one series")
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellStr("Sheet1", "A1", "Category")
	for i, cat := range c.Categories {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if c.Type == "scatter" || c.Type == "bubble" {
			v, err := strconv.ParseFloat(cat, 64)
			if err != nil || !finite(v) {
				return "", fmt.Errorf("scatter categories must contain numeric X coordinates")
			}
			if err := f.SetCellFloat("Sheet1", cell, v, -1, 64); err != nil {
				return "", err
			}
		} else if err := f.SetCellStr("Sheet1", cell, cat); err != nil {
			return "", err
		}
	}
	var series []excelize.ChartSeries
	for j, s := range c.Series {
		if len(s.Values) != len(c.Categories) {
			return "", fmt.Errorf("series %q has different category/value counts", s.Name)
		}
		col, _ := excelize.ColumnNumberToName(j + 2)
		f.SetCellStr("Sheet1", col+"1", s.Name)
		for i, v := range s.Values {
			if !finite(v) {
				return "", fmt.Errorf("non-finite chart value")
			}
			if err := f.SetCellFloat("Sheet1", fmt.Sprintf("%s%d", col, i+2), v, -1, 64); err != nil {
				return "", err
			}
		}
		series = append(series, excelize.ChartSeries{Name: "Sheet1!$" + col + "$1", Categories: fmt.Sprintf("Sheet1!$A$2:$A$%d", len(c.Categories)+1), Values: fmt.Sprintf("Sheet1!$%s$2:$%s$%d", col, col, len(s.Values)+1)})
		if c.Type == "bubble" {
			if len(s.Sizes) != len(s.Values) {
				return "", fmt.Errorf("bubble sizes must match values")
			}
			sizeCol, _ := excelize.ColumnNumberToName(len(c.Series) + j + 2)
			for i, v := range s.Sizes {
				if !finite(v) || v < 0 {
					return "", fmt.Errorf("bubble sizes must be finite and nonnegative")
				}
				if err := f.SetCellFloat("Sheet1", fmt.Sprintf("%s%d", sizeCol, i+2), v, -1, 64); err != nil {
					return "", err
				}
			}
			series[len(series)-1].Sizes = fmt.Sprintf("Sheet1!$%s$2:$%s$%d", sizeCol, sizeCol, len(s.Sizes)+1)
		}

	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Capture editable source cells before chart generation. DeleteChart leaves
	// drawing/chart parts behind; those belong only to the presentation chart.
	data, err := f.WriteToBuffer()
	if err != nil {
		return "", err
	}
	chartOptions := excelize.Chart{Type: typ, Series: series, Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: c.Title, Font: &excelize.Font{Family: font, Size: 18}}}}}
	if c.Options != nil {
		chartOptions = *c.Options
		chartOptions.Type = typ
		chartOptions.Series = series
	}
	if err := f.AddChart("Sheet1", "A20", &chartOptions); err != nil {
		return "", err
	}
	raw, err := f.WriteToBuffer()
	if err != nil {
		return "", err
	}
	p, err := opc.Read(raw.Bytes())
	if err != nil {
		return "", err
	}
	if err = office.RefreshWorkbookChartCaches(p); err != nil {
		return "", err
	}
	chart, err := p.XML("xl/charts/chart1.xml")
	if err != nil {
		return "", err
	}
	root := chart.Root()
	// reference presentation library's column/bar authoring leaves varyColors absent (false).
	// Excelize enables it, which makes single-series legends use categories.
	if c.Type == "column" || c.Type == "bar" {
		// Match reference presentation library's chart-space text default for authored bar plots.
		// Axis and legend text otherwise inherit an application-specific size.
		if office.Child(root, office.NSC, "txPr") == nil {
			text := etree.NewElement("txPr")
			text.CreateElement("a:bodyPr")
			text.CreateElement("a:lstStyle")
			paragraph := text.CreateElement("a:p")
			paragraph.CreateElement("a:pPr").CreateElement("a:defRPr").CreateAttr("sz", "1800")
			paragraph.CreateElement("a:endParaRPr").CreateAttr("lang", "en-US")
			office.InsertBefore(root, text, []string{"externalData", "printSettings", "userShapes", "extLst"})
		}
		for _, plot := range office.All(root, office.NSC, "barChart") {
			if vary := office.Child(plot, office.NSC, "varyColors"); vary != nil {
				plot.RemoveChild(vary)
			}
		}
	}
	if c.InheritFormatting {
		if c.Type != "column" || c.Title != "" {
			return "", fmt.Errorf("inherited chart formatting requires an untitled column chart")
		}
		chart, err = inheritedColumnChart(c)
		if err != nil {
			return "", err
		}
		root = chart.Root()
	}
	root.CreateAttr("xmlns:r", rns)
	ext := root.CreateElement("c:externalData") // Excelize's chart namespace may be the default namespace.
	ext.Space = ""
	ext.Tag = "externalData"
	ext.CreateAttr("r:id", "rId1")
	ext.CreateElement("autoUpdate").CreateAttr("val", "0")
	root.RemoveChild(ext)
	office.InsertBefore(root, ext, []string{"printSettings", "userShapes", "extLst"})
	b.charts++
	name := fmt.Sprintf("chart%d.xml", b.charts)
	embedded := fmt.Sprintf("Microsoft_Excel_Sheet%d.xlsx", b.charts)
	if c.InheritFormatting {
		var prefix func(*etree.Element)
		prefix = func(e *etree.Element) {
			if e.Space == "" {
				e.Space = "c"
			}
			for _, child := range e.ChildElements() {
				prefix(child)
			}
		}
		root.RemoveAttr("xmlns")
		prefix(root)
		root.Attr = append([]etree.Attr{{Space: "xmlns", Key: "c", Value: cns}}, root.Attr...)
	}
	xmlData, err := chart.WriteToBytes()
	if err != nil {
		return "", err
	}
	if c.InheritFormatting {
		xmlData = append([]byte(referenceXMLDeclaration), xmlData...)
	}
	b.add("ppt/charts/"+name, "application/vnd.openxmlformats-officedocument.drawingml.chart+xml", string(xmlData))
	b.p.Set("ppt/charts/_rels/"+name+".rels", relationships(rel("rId1", "package", "../embeddings/"+embedded)))
	if categoryWorkbookSupported(c) {
		raw, err := categoryWorkbook(c, b.clock())
		if err != nil {
			return "", err
		}
		data = bytes.NewBuffer(raw)
	}
	b.p.Set("ppt/embeddings/"+embedded, data.Bytes())
	return name, nil
}
