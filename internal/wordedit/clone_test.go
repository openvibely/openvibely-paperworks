package wordedit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestCloneComponentPreservesBlocksAndBookmarkLinks(t *testing.T) {
	source := mk(t, "component", `<w:p><w:r><w:t>Before</w:t></w:r></w:p><w:p><w:bookmarkStart w:id="0" w:name="Topic"/><w:r><w:rPr><w:b/></w:rPr><w:t>Clone me</w:t></w:r><w:bookmarkEnd w:id="0"/><w:hyperlink w:anchor="Topic"><w:r><w:t>Link</w:t></w:r></w:hyperlink></w:p><w:tbl><w:tblPr/><w:tblGrid><w:gridCol w:w="4000"/></w:tblGrid><w:tr><w:tc><w:p><w:r><w:t>Table</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p><w:r><w:t>After</w:t></w:r></w:p>`, nil)
	raw, _ := os.ReadFile(source)
	out := filepath.Join(t.TempDir(), "clone.docx")
	if _, err := Edit(source, out, map[string]any{"operation": "clone-component", "start": float64(1), "end": float64(2), "before": float64(0)}, ""); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(raw, after) {
		t.Fatal("source changed")
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := p.XML("word/document.xml")
	text := office.Text(d.Root(), nsW, "t")
	if text != "Clone meLinkTableBeforeClone meLinkTableAfter" {
		t.Fatal(text)
	}
	bookmarks := office.All(d.Root(), nsW, "bookmarkStart")
	if len(bookmarks) != 2 || getW(bookmarks[0], "name") == getW(bookmarks[1], "name") || getW(bookmarks[0], "id") == getW(bookmarks[1], "id") {
		t.Fatal("bookmark collision")
	}
	links := office.All(d.Root(), nsW, "hyperlink")
	if getW(links[0], "anchor") != getW(bookmarks[0], "name") {
		t.Fatal("clone link does not target clone")
	}
	if len(office.All(d.Root(), nsW, "b")) != 2 {
		t.Fatal("lost formatting")
	}
}

func TestCloneComponentCopiesImageAndConflictingStyle(t *testing.T) {
	target := mk(t, "target", `<w:p><w:pPr><w:pStyle w:val="Component"/></w:pPr><w:r><w:t>Keep</w:t></w:r></w:p>`, map[string]string{"word/styles.xml": `<w:styles ` + wNS + `><w:style w:type="paragraph" w:styleId="Component"><w:name w:val="Destination"/><w:rPr><w:color w:val="000000"/></w:rPr></w:style></w:styles>`})
	template := mk(t, "source", `<w:p><w:pPr><w:pStyle w:val="Component"/></w:pPr><w:r><w:t>Template</w:t><w:drawing><wp:inline><wp:extent cx="914400" cy="914400"/><wp:docPr id="1" name="Image"/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:blipFill><a:blip r:embed="rId100"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p><w:p><w:r><w:t>Not selected</w:t></w:r></w:p>`, map[string]string{"word/styles.xml": `<w:styles ` + wNS + `><w:style w:type="paragraph" w:styleId="Component"><w:name w:val="Source"/><w:rPr><w:color w:val="FF0000"/></w:rPr></w:style></w:styles>`})
	p, _ := opc.Open(template)
	p.Set("word/media/test.png", []byte("test-image-data"))
	if err := p.EnsureContentType("word/media/test.png", "image/png"); err != nil {
		t.Fatal(err)
	}
	if err := addRelWithID(p, "word/document.xml", "rId100", nsR+"/image", "media/test.png"); err != nil {
		t.Fatal(err)
	}
	raw, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(template, raw, 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.docx")
	if _, err := Edit(target, out, map[string]any{"operation": "clone-component", "path": template, "start": float64(0), "end": float64(0)}, ""); err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(out)
	d, _ := p.XML("word/document.xml")
	if office.Text(d.Root(), nsW, "t") != "KeepTemplate" {
		t.Fatal("copied unselected block")
	}
	styles := office.All(d.Root(), nsW, "pStyle")
	if getW(styles[0], "val") != "Component" || getW(styles[1], "val") == "Component" {
		t.Fatal("style collision")
	}
	blips := office.All(d.Root(), office.NSA, "blip")
	if len(blips) != 1 {
		t.Fatal("missing image")
	}
	rid := blips[0].SelectAttrValue("r:embed", "")
	rels, _ := p.Rels("word/document.xml")
	found := false
	for _, r := range rels {
		if r.ID == rid {
			found = true
			if string(p.Get(opc.Resolve("word/document.xml", r.Target))) != "test-image-data" {
				t.Fatal("lost related asset")
			}
		}
	}
	if !found {
		t.Fatal("missing image relationship")
	}
	st, _ := p.XML("word/styles.xml")
	found = false
	for _, s := range office.Children(st.Root(), nsW, "style") {
		if getW(s, "styleId") == getW(styles[1], "val") {
			found = true
			colors := office.All(s, nsW, "color")
			if len(colors) != 1 || getW(colors[0], "val") != "FF0000" {
				t.Fatal("source style lost")
			}
		}
	}
	if !found {
		t.Fatal("imported style missing")
	}
}

func TestCloneComponentRejectsIncompleteReferences(t *testing.T) {
	for _, fragment := range []string{`<w:bookmarkStart w:id="0" w:name="Cross"/>`, `<w:r><w:footnoteReference w:id="1"/></w:r>`, `<w:pPr><w:sectPr/></w:pPr>`, `<w:r><w:fldChar w:fldCharType="begin"/></w:r>`} {
		source := mk(t, "invalid", `<w:p>`+fragment+`<w:r><w:t>Text</w:t></w:r></w:p>`, nil)
		out := filepath.Join(t.TempDir(), "out.docx")
		if _, err := Edit(source, out, map[string]any{"operation": "clone-component", "start": float64(0), "end": float64(0)}, ""); err == nil {
			t.Fatal("unsupported dependency accepted", fragment)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("failed clone published output")
		}
	}
}

func TestCloneRelatedPartPreservesExternalRelationshipIDs(t *testing.T) {
	source := mk(t, "related", `<w:p/>`, nil)
	target := mk(t, "destination", `<w:p/>`, nil)
	sr, _ := os.ReadFile(source)
	dr, _ := os.ReadFile(target)
	src, err := Open(sr)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := Open(dr)
	if err != nil {
		t.Fatal(err)
	}
	src.Pkg.Set("word/charts/chart1.xml", []byte(`<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><c:externalData r:id="rId9"/></c:chartSpace>`))
	if err := src.Pkg.EnsureContentType("word/charts/chart1.xml", "application/vnd.openxmlformats-officedocument.drawingml.chart+xml"); err != nil {
		t.Fatal(err)
	}
	if err := addRelWithMode(src.Pkg, "word/charts/chart1.xml", "rId9", nsR+"/package", "https://example.test/data.xlsx", "External"); err != nil {
		t.Fatal(err)
	}
	part, err := copyPartTree(dst, src, "word/charts/chart1.xml", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	rels, err := dst.Pkg.Rels(part)
	if err != nil || len(rels) != 1 || rels[0].ID != "rId9" || rels[0].TargetMode != "External" || rels[0].Target != "https://example.test/data.xlsx" {
		t.Fatal("related external link changed", rels, err)
	}
}

func TestClonePinsConflictingDefaultParagraphStyle(t *testing.T) {
	styles := func(color string) map[string]string {
		return map[string]string{"word/styles.xml": `<w:styles ` + wNS + `><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:rPr><w:color w:val="` + color + `"/></w:rPr></w:style></w:styles>`}
	}
	target := mk(t, "dst", `<w:p><w:r><w:t>Destination</w:t></w:r></w:p>`, styles("000000"))
	source := mk(t, "src", `<w:p><w:r><w:t>Source</w:t></w:r></w:p>`, styles("FF0000"))
	out := filepath.Join(t.TempDir(), "cloned.docx")
	if _, err := Edit(target, out, map[string]any{"operation": "clone-component", "path": source, "start": float64(0), "end": float64(0)}, ""); err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("word/document.xml")
	stylesUsed := office.All(d.Root(), nsW, "pStyle")
	if len(stylesUsed) != 1 || getW(stylesUsed[0], "val") == "Normal" {
		t.Fatal("source paragraph inherited destination default")
	}
}

func TestCloneKeepsDestinationDefaultWhenSourceHasDifferentStyleID(t *testing.T) {
	styles := func(id, color string) map[string]string {
		return map[string]string{"word/styles.xml": `<w:styles ` + wNS + `><w:style w:type="paragraph" w:default="1" w:styleId="` + id + `"><w:name w:val="` + id + `"/><w:rPr><w:color w:val="` + color + `"/></w:rPr></w:style></w:styles>`}
	}
	target := mk(t, "dst", `<w:p><w:r><w:t>Original</w:t></w:r></w:p>`, styles("DestinationDefault", "000000"))
	source := mk(t, "src", `<w:p><w:r><w:t>Copied</w:t></w:r></w:p>`, styles("SourceDefault", "FF0000"))
	out := filepath.Join(t.TempDir(), "cloned.docx")
	if _, err := Edit(target, out, map[string]any{"operation": "clone-component", "path": source, "start": float64(0), "end": float64(0)}, ""); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	st := d.styles()
	if st.def != "DestinationDefault" {
		t.Fatalf("destination default changed to %s", st.def)
	}
	defaults := 0
	for _, s := range st.byID {
		if office.Truthy(getW(s, "default"), false) {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("got %d default styles", defaults)
	}
	paragraphs := office.Children(d.Body(), nsW, "p")
	if child(paragraphs[0], "pPr") != nil {
		t.Fatal("original paragraph changed")
	}
	sourceStyle := getW(child(child(paragraphs[1], "pPr"), "pStyle"), "val")
	if sourceStyle != "SourceDefault" {
		t.Fatal("clone did not retain its source style", sourceStyle)
	}
	for id, want := range map[string]string{"DestinationDefault": "000000", "SourceDefault": "FF0000"} {
		cs := office.All(st.byID[id], nsW, "color")
		if len(cs) != 1 || getW(cs[0], "val") != want {
			t.Fatalf("style %s lost its color", id)
		}
	}
}

func TestCloneImportsStyleNumberingEvenWhenStyleXMLMatches(t *testing.T) {
	for _, inherit := range []bool{false, true} {
		t.Run(fmt.Sprint(inherit), func(t *testing.T) {
			style := "Numbered"
			if inherit {
				style = "Derived"
			}
			fixture := func(name, format string) string {
				file := mk(t, name, `<w:p><w:pPr><w:pStyle w:val="`+style+`"/></w:pPr><w:r><w:t>`+name+`</w:t></w:r></w:p>`, map[string]string{
					"word/styles.xml":    `<w:styles ` + wNS + `><w:style w:type="paragraph" w:styleId="Numbered"><w:name w:val="Numbered"/><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr></w:style><w:style w:type="paragraph" w:styleId="Derived"><w:name w:val="Derived"/><w:basedOn w:val="Numbered"/></w:style></w:styles>`,
					"word/numbering.xml": `<w:numbering ` + wNS + `><w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="` + format + `"/><w:lvlText w:val="%1."/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num></w:numbering>`,
				})
				p, err := opc.Open(file)
				if err != nil {
					t.Fatal(err)
				}
				rels, err := p.Rels("word/document.xml")
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, r := range rels {
					if r.Type == nsR+"/numbering" {
						found = true
					}
				}
				if !found {
					if _, err := p.AddRel("word/document.xml", nsR+"/numbering", "numbering.xml", false); err != nil {
						t.Fatal(err)
					}
				}
				if err := p.EnsureContentType("word/numbering.xml", ctBase+"numbering+xml"); err != nil {
					t.Fatal(err)
				}
				raw, err := p.Bytes()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
				return file
			}
			dst, src := fixture("Destination", "decimal"), fixture("Source", "upperRoman")
			out := filepath.Join(t.TempDir(), "cloned.docx")
			if _, err := Edit(dst, out, map[string]any{"operation": "clone-component", "path": src, "start": float64(0), "end": float64(0)}, ""); err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(out)
			d, err := Open(raw)
			if err != nil {
				t.Fatal(err)
			}
			st := d.styles()
			numbers, err := d.Root("word/numbering.xml")
			if err != nil {
				t.Fatal(err)
			}
			for i, para := range office.Children(d.Body(), nsW, "p") {
				// Resolve direct numbering first, then inherited paragraph-style numbering.
				props := child(para, "pPr")
				id := getW(child(child(props, "numPr"), "numId"), "val")
				sid := getW(child(props, "pStyle"), "val")
				for j := 0; id == "" && sid != "" && j < 10; j++ {
					s := st.byID[sid]
					if s == nil {
						t.Fatal("missing style", sid)
					}
					id = getW(child(child(child(s, "pPr"), "numPr"), "numId"), "val")
					sid = getW(child(s, "basedOn"), "val")
				}
				abstract := ""
				for _, n := range office.Children(numbers, nsW, "num") {
					if getW(n, "numId") == id {
						abstract = getW(child(n, "abstractNumId"), "val")
					}
				}
				got := ""
				for _, a := range office.Children(numbers, nsW, "abstractNum") {
					if getW(a, "abstractNumId") == abstract {
						got = getW(child(child(a, "lvl"), "numFmt"), "val")
					}
				}
				want := "decimal"
				if i == 1 {
					want = "upperRoman"
				}
				if got != want {
					t.Fatalf("paragraph %d numbering=%s, want %s", i, got, want)
				}
			}
		})
	}
}
