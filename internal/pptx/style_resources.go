package pptx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/xmlresource"
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
	raw, d, err := xmlresource.Read(base, name, namespace, tag)
	if err != nil {
		var rootErr *xmlresource.RootError
		if errors.As(err, &rootErr) {
			return nil, fmt.Errorf("style resource must have {%s}%s root", namespace, tag)
		}
		return nil, err
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
	forEachTextParagraph(e, func(p Paragraph, runs []Run) {
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
			if r.UnderlineStyle != "" {
				b.WriteString(` u="` + esc(r.UnderlineStyle) + `"`)
			} else if r.Underline {
				b.WriteString(` u="sng"`)
			}
			ink := e.Color
			if r.Color != "" {
				ink = r.Color
			}
			if ink == "" && r.Font == "" {
				b.WriteString(`/>`)
			} else {
				b.WriteString(`>`)
				if ink != "" {
					b.WriteString(`<a:solidFill><a:srgbClr val="` + ink + `"/></a:solidFill>`)
				}
				if r.Font != "" {
					b.WriteString(`<a:latin typeface="` + esc(r.Font) + `"/>`)
				}
				b.WriteString(`</a:rPr>`)
			}
			b.WriteString(`<a:t>` + esc(r.Text) + `</a:t></a:r>`)
		}
		b.WriteString(`</a:p>`)
	})
	return b.String()
}
