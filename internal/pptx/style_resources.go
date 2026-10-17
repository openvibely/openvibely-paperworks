package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"os"
	"path/filepath"
	"strings"
)

// StyleResources supply reusable PresentationML styles; slide content remains authored.
type StyleResources struct {
	// TemplateLayout selects one of the eleven pinned default layouts (0..10).
	// It cannot be combined with a custom Master or Layout.
	TemplateLayout *int   `json:"template_layout,omitempty"`
	TableStyles    string `json:"table_styles"`
	Theme          string `json:"theme"`
	Master         string `json:"master"`
	Layout         string `json:"layout"`
	DefaultText    string `json:"default_text"`
}

func readStyleResource(base, name, namespace, tag string) ([]byte, error) {
	if !filepath.IsAbs(name) {
		name = filepath.Join(base, name)
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	d := etree.NewDocument()
	if err = d.ReadFromBytes(raw); err != nil {
		return nil, err
	}
	if d.Root() == nil || d.Root().Tag != tag || d.Root().NamespaceURI() != namespace {
		return nil, fmt.Errorf("style resource must have {%s}%s root", namespace, tag)
	}
	// A resource cannot introduce undeclared relationships. The sole
	// supported master link is the generated layout relationship rId1.
	var check func(*etree.Element) error
	check = func(e *etree.Element) error {
		for _, a := range e.Attr {
			if a.NamespaceURI() == rns && !(tag == "sldMaster" && e.Tag == "sldLayoutId" && a.Key == "id" && a.Value == "rId1") {
				return fmt.Errorf("unsupported relationship in style resource")
			}
		}
		for _, c := range e.ChildElements() {
			if err := check(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := check(d.Root()); err != nil {
		return nil, err
	}
	if tag == "defaultTextStyle" {
		fragment := etree.NewDocument()
		fragment.SetRoot(d.Root().Copy())
		return fragment.WriteToBytes()
	}
	return raw, nil
}
func inheritedTextBody(e Element) string {
	var b strings.Builder
	b.WriteString(`<a:bodyPr/><a:lstStyle/>`)
	ps := e.Paragraphs
	if len(ps) == 0 {
		for _, text := range strings.Split(e.Text, "\n") {
			ps = append(ps, Paragraph{Text: text})
		}
	}
	for _, p := range ps {
		b.WriteString(`<a:p>`)
		if p.Bullet || p.Level != 0 || e.Align != "" {
			fmt.Fprintf(&b, `<a:pPr lvl="%d"`, p.Level)
			if e.Align != "" {
				fmt.Fprintf(&b, ` algn="%s"`, e.Align)
			}
			b.WriteString(`>`)
			if p.Bullet {
				b.WriteString(`<a:buChar char="•"/>`)
			}
			b.WriteString(`</a:pPr>`)
		}
		runs := p.Runs
		if len(runs) == 0 {
			runs = []Run{{Text: p.Text}}
		}
		for _, r := range runs {
			b.WriteString(`<a:r><a:rPr`)
			size := e.FontSize
			if r.FontSize != 0 {
				size = r.FontSize
			}
			if size != 0 {
				fmt.Fprintf(&b, ` sz="%d"`, int(size*100))
			}
			if r.Bold != nil {
				if *r.Bold {
					b.WriteString(` b="1"`)
				} else {
					b.WriteString(` b="0"`)
				}
			} else if e.Bold || p.Bold {
				b.WriteString(` b="1"`)
			}
			if r.Italic {
				b.WriteString(` i="1"`)
			}
			if r.Underline {
				b.WriteString(` u="sng"`)
			}
			ink := e.Color
			if r.Color != "" {
				ink = r.Color
			}
			if ink == "" {
				b.WriteString(`/>`)
			} else {
				b.WriteString(`><a:solidFill><a:srgbClr val="` + ink + `"/></a:solidFill></a:rPr>`)
			}
			b.WriteString(`<a:t>` + esc(r.Text) + `</a:t></a:r>`)
		}
		b.WriteString(`</a:p>`)
	}
	return b.String()
}
