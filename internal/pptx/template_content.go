package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"strings"
)

const referenceXMLDeclaration = "<?xml version='1.0' encoding='UTF-8' standalone='yes'?>\n"

// Finalize default-template content using the template's namespace and
// placeholder conventions. Custom-layout authoring retains its existing path.
func (b *builder) finalizeTemplateContent(s Spec) error {
	write := func(name, declaration string, change func(*etree.Element)) error {
		d, err := b.p.XML(name)
		if err != nil {
			return err
		}
		change(d.Root())
		clean := etree.NewDocument()
		clean.SetRoot(d.Root().Copy())
		raw, err := clean.WriteToBytes()
		if err != nil {
			return err
		}
		b.p.Set(name, append([]byte(declaration), raw...))
		return nil
	}
	raw, err := defaultResources.ReadFile("assets/default/docProps/core.xml")
	if err != nil {
		return err
	}
	b.p.Set("docProps/core.xml", raw)
	if err := write("docProps/core.xml", referenceXMLDeclaration, func(r *etree.Element) {
		if s.Title != "" {
			r.SelectElement("dc:title").SetText(s.Title)
		}
	}); err != nil {
		return err
	}
	if err := write("ppt/theme/theme1.xml", "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\r\n", func(*etree.Element) {}); err != nil {
		return err
	}
	if err := write("ppt/presentation.xml", "<?xml version=\"1.0\" encoding=\"UTF-8\"?>", func(r *etree.Element) {
		r.Attr = append([]etree.Attr{{Space: "xmlns", Key: "a", Value: ans}, {Space: "xmlns", Key: "r", Value: rns}, {Space: "xmlns", Key: "p", Value: pns}}, r.Attr[3:]...)
		r.CreateAttr("saveSubsetFonts", "1")
		r.CreateAttr("autoCompressPictures", "0")
		r.SelectElement("p:sldSz").CreateAttr("type", "screen4x3")
		n := r.FindElement("p:notesMasterIdLst/p:notesMasterId")
		if n != nil {
			id := n.SelectAttrValue("r:id", "")
			n.Attr = nil
			n.CreateAttr("xmlns:r", rns)
			n.CreateAttr("r:id", id)
		}
		if t := r.SelectElement("p:defaultTextStyle"); t != nil {
			t.RemoveAttr("xmlns:p")
			t.RemoveAttr("xmlns:a")
			t.RemoveAttr("xmlns:r")
		}
	}); err != nil {
		return err
	}
	for i, sl := range s.Slides {
		notesName := fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i+1)
		blank, err := defaultResources.ReadFile("assets/default/ppt/notes.xml")
		if err != nil {
			return err
		}
		b.p.Set(notesName, blank)
		if err := write(notesName, referenceXMLDeclaration, func(r *etree.Element) {
			body := r.FindElement(".//p:txBody")
			for _, p := range body.SelectElements("a:p") {
				body.RemoveChild(p)
			}
			for _, line := range strings.Split(sl.Notes, "\n") {
				p := body.CreateElement("a:p")
				if line != "" {
					p.CreateElement("a:r").CreateElement("a:t").SetText(line)
				}
			}
		}); err != nil {
			return err
		}

		name := fmt.Sprintf("ppt/slides/slide%d.xml", i+1)
		if err := write(name, referenceXMLDeclaration, func(r *etree.Element) {
			r.Attr = []etree.Attr{{Space: "xmlns", Key: "a", Value: ans}, {Space: "xmlns", Key: "p", Value: pns}, {Space: "xmlns", Key: "r", Value: rns}}
			cs := r.SelectElement("p:cSld")
			if sl.Background == "" {
				if bg := cs.SelectElement("p:bg"); bg != nil {
					cs.RemoveChild(bg)
				}
			}
			tree := cs.SelectElement("p:spTree")
			tree.SelectElement("p:grpSpPr").Child = nil
			shapes := tree.ChildElements()[2:]
			for j, e := range sl.Elements {
				shape := shapes[j]
				nv := shape.ChildElements()[0]
				prop := nv.SelectElement("p:cNvPr")
				if e.Description == "" {
					prop.RemoveAttr("descr")
				}
				if e.Name == "" {
					name := map[string]string{"text": "TextBox", "table": "Table", "chart": "Chart", "image": "Picture", "shape": "AutoShape"}[e.Type]
					if e.Placeholder != "" {
						name = "Title"
					}
					prop.SelectAttr("name").Value = fmt.Sprintf("%s %d", name, j+1)
				}
				if e.Type == "chart" || e.Type == "table" {
					nv.SelectElement("p:cNvGraphicFramePr").CreateElement("a:graphicFrameLocks").CreateAttr("noGrp", "1")
				}
				if e.Placeholder != "" {
					nv.SelectElement("p:cNvSpPr").CreateElement("a:spLocks").CreateAttr("noGrp", "1")
					if e.InheritFormatting && e.Fill == "" && e.Shape == "" {
						sp := shape.SelectElement("p:spPr")
						for _, c := range sp.ChildElements() {
							if c.Tag != "xfrm" {
								sp.RemoveChild(c)
							}
						}
					}
				}
			}
		}); err != nil {
			return err
		}
	}
	return nil
}
