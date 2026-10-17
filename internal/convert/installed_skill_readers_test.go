package convert

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// reference presentation library has_chart/has_table inspect the graphic-frame data URI,
// and slide.shapes visits only direct children of spTree.
func referenceSkillFrames(root *etree.Element, uri string) []*etree.Element {
	tree := office.Child(office.Child(root, office.NSP, "cSld"), office.NSP, "spTree")
	var result []*etree.Element
	for _, frame := range office.Children(tree, office.NSP, "graphicFrame") {
		data := office.Child(office.Child(frame, office.NSA, "graphic"), office.NSA, "graphicData")
		if data != nil && data.SelectAttrValue("uri", "") == uri {
			if uri == office.NSC {
				result = append(result, office.Child(data, office.NSC, "chart"))
			} else {
				result = append(result, data)
			}
		}
	}
	return result
}

func referenceSkillNotes(t *testing.T, pkg *opc.Package, slide string) string {
	t.Helper()
	rels, err := pkg.Rels(slide)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range rels {
		if !strings.HasSuffix(rel.Type, "/notesSlide") {
			continue
		}
		doc, err := pkg.XML(opc.Resolve(slide, rel.Target))
		if err != nil {
			t.Fatal(err)
		}
		if body := referenceSkillNotesBody(doc.Root()); body != nil {
			return referenceRuntimeFrameText(body)
		}
	}
	t.Fatal("notes body placeholder missing")
	return ""
}

// reference DOCX library BabelFish.internal2ui maps only these built-in aliases.
func referenceSkillUIStyleName(style *etree.Element) string {
	name := office.AttrV(office.Child(style, office.NSW, "name"), office.NSW, "val")
	switch name {
	case "caption", "footer", "header", "heading 1", "heading 2", "heading 3", "heading 4", "heading 5", "heading 6", "heading 7", "heading 8", "heading 9":
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}

func referenceSkillStyleName(root *etree.Element, id string) string {
	for _, style := range office.Children(root, office.NSW, "style") {
		if id != "" && office.AttrV(style, office.NSW, "styleId") == id && office.AttrV(style, office.NSW, "type") == "paragraph" {
			return referenceSkillUIStyleName(style)
		}
	}
	// CT_Styles.default_for selects the last default in document order.
	var fallback *etree.Element
	for _, style := range office.Children(root, office.NSW, "style") {
		value := office.AttrV(style, office.NSW, "default")
		if office.AttrV(style, office.NSW, "type") == "paragraph" && (value == "1" || value == "true" || value == "on") {
			fallback = style
		}
	}
	return referenceSkillUIStyleName(fallback)
}

func TestReferenceSkillReaderMembership(t *testing.T) {
	doc := etree.NewDocument()
	raw := `<p:sld xmlns:p="` + office.NSP + `" xmlns:a="` + office.NSA + `" xmlns:c="` + office.NSC + `"><p:cSld><p:spTree><p:grpSp><p:graphicFrame><a:graphic><a:graphicData uri="` + office.NSC + `"><c:chart/></a:graphicData></a:graphic></p:graphicFrame></p:grpSp><p:graphicFrame><a:graphic><a:graphicData uri="wrong"><c:chart/><a:tbl/></a:graphicData></a:graphic></p:graphicFrame><p:graphicFrame><a:graphic><a:graphicData uri="` + office.NSC + `"><c:chart/></a:graphicData></a:graphic></p:graphicFrame><p:graphicFrame><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl/></a:graphicData></a:graphic></p:graphicFrame></p:spTree></p:cSld></p:sld>`
	if err := doc.ReadFromString(raw); err != nil {
		t.Fatal(err)
	}
	if got := len(referenceSkillFrames(doc.Root(), office.NSC)); got != 1 {
		t.Fatalf("charts %d", got)
	}
	if got := len(referenceSkillFrames(doc.Root(), "http://schemas.openxmlformats.org/drawingml/2006/table")); got != 1 {
		t.Fatalf("tables %d", got)
	}
	styles := etree.NewDocument()
	if err := styles.ReadFromString(`<w:styles xmlns:w="` + office.NSW + `"><w:style w:type="paragraph" w:styleId="Normal" w:default="1"><w:name w:val="Normal"/></w:style><w:style w:type="paragraph" w:styleId="RenamedID"><w:name w:val="Title"/></w:style><w:style w:type="character" w:styleId="Title"><w:name w:val="Wrong"/></w:style></w:styles>`); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"RenamedID": "Title", "Title": "Normal", "missing": "Normal", "": "Normal"} {
		if got := referenceSkillStyleName(styles.Root(), id); got != want {
			t.Errorf("%q: %q != %q", id, got, want)
		}
	}
}

func TestReferenceSkillStyleAliasesAndDefaults(t *testing.T) {
	doc := etree.NewDocument()
	raw := `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:type="paragraph" w:styleId="First" w:default="1"><w:name w:val="Normal"/></w:style><w:style w:type="paragraph" w:styleId="Last" w:default="true"><w:name w:val="heading 1"/></w:style><w:style w:type="paragraph" w:styleId="Caption"><w:name w:val="caption"/></w:style><w:style w:type="paragraph" w:styleId="Custom"><w:name w:val="heading 10"/></w:style><w:style w:type="character" w:styleId="Character" w:default="1"><w:name w:val="Wrong"/></w:style></w:styles>`
	if err := doc.ReadFromString(raw); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"": "Heading 1", "missing": "Heading 1", "Character": "Heading 1", "First": "Normal", "Last": "Heading 1", "Caption": "Caption", "Custom": "heading 10"} {
		if got := referenceSkillStyleName(doc.Root(), id); got != want {
			t.Errorf("%q: %q != %q", id, got, want)
		}
	}
}

// referenceSkillSlides follows the package officeDocument relationship and the
// presentation sldIdLst order. Orphan slide parts are not presentation members.
// This scoped reader does not certify general OPC descriptor validation.
func referenceSkillSlides(t *testing.T, pkg *opc.Package) []string {
	t.Helper()
	const relNS = "http://schemas.openxmlformats.org/package/2006/relationships"
	const officeRel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	resolve := func(owner, target string) string {
		if strings.HasPrefix(target, "/") {
			return strings.TrimPrefix(target, "/")
		}
		return path.Clean(path.Join(path.Dir(owner), target))
	}
	root, err := pkg.XML("_rels/.rels")
	if err != nil {
		t.Fatal(err)
	}
	var presentation string
	for _, rel := range office.Children(root.Root(), relNS, "Relationship") {
		if rel.SelectAttrValue("Type", "") == officeRel+"/officeDocument" {
			if rel.SelectAttrValue("TargetMode", "") == "External" {
				t.Fatal("external presentation")
			}
			presentation = resolve("", rel.SelectAttrValue("Target", ""))
		}
	}
	if presentation == "" {
		t.Fatal("missing presentation relationship")
	}
	doc, err := pkg.XML(presentation)
	if err != nil {
		t.Fatal(err)
	}
	rels, err := pkg.XML(path.Join(path.Dir(presentation), "_rels", path.Base(presentation)+".rels"))
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]string{}
	for _, rel := range office.Children(rels.Root(), relNS, "Relationship") {
		if rel.SelectAttrValue("Type", "") == officeRel+"/slide" && rel.SelectAttrValue("TargetMode", "") != "External" {
			targets[rel.SelectAttrValue("Id", "")] = resolve(presentation, rel.SelectAttrValue("Target", ""))
		}
	}
	var slides []string
	for _, id := range office.Children(office.Child(doc.Root(), office.NSP, "sldIdLst"), office.NSP, "sldId") {
		target := targets[office.AttrV(id, officeRel, "id")]
		if target == "" {
			t.Fatal("missing slide relationship")
		}
		if _, err := pkg.XML(target); err != nil {
			t.Fatal(err)
		}
		slides = append(slides, target)
	}
	return slides
}

// Match reference presentation library CT_NumDataSource: count and indexed points, not XML order.
func referenceSkillSeriesValues(value *etree.Element) ([]*float64, error) {
	counts := office.All(value, office.NSC, "ptCount")
	if len(counts) == 0 {
		return nil, nil
	}
	count, err := strconv.Atoi(counts[0].SelectAttrValue("val", ""))
	if err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, nil
	}
	result := make([]*float64, count)
	points := office.All(value, office.NSC, "pt")
	for idx := range result {
		for _, point := range points {
			// XPath's numeric comparison accepts numeric spellings of the index.
			n, err := strconv.ParseFloat(point.SelectAttrValue("idx", ""), 64)
			if err != nil || n != float64(idx) {
				continue
			}
			v := office.Child(point, office.NSC, "v")
			if v == nil {
				return nil, fmt.Errorf("missing point value")
			}
			number, err := strconv.ParseFloat(v.Text(), 64)
			if err != nil {
				return nil, err
			}
			result[idx] = &number
			break
		}
	}
	return result, nil
}

func TestReferenceSkillIndexedChartValues(t *testing.T) {
	for _, tc := range []struct {
		body string
		want []string
	}{
		{`<c:ptCount val="2"/><c:pt idx="1"><c:v>80</c:v></c:pt><c:pt idx="0"><c:v>120</c:v></c:pt>`, []string{"120", "80"}},
		{`<c:ptCount val="3"/><c:pt idx="1"><c:v>80</c:v></c:pt>`, []string{"nil", "80", "nil"}},
		{`<c:pt idx="0"><c:v>120</c:v></c:pt>`, nil},
		{`<c:ptCount val="1"/><c:pt idx="0"><c:v>120</c:v></c:pt><c:pt idx="0"><c:v>80</c:v></c:pt>`, []string{"120"}},
	} {
		d := etree.NewDocument()
		if err := d.ReadFromString(`<c:val xmlns:c="` + office.NSC + `"><c:numRef><c:numCache>` + tc.body + `</c:numCache></c:numRef></c:val>`); err != nil {
			t.Fatal(err)
		}
		got, err := referenceSkillSeriesValues(d.Root())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("length %d != %d", len(got), len(tc.want))
		}
		for i, v := range got {
			text := "nil"
			if v != nil {
				text = strconv.FormatFloat(*v, 'g', -1, 64)
			}
			if text != tc.want[i] {
				t.Errorf("point %d: %s != %s", i, text, tc.want[i])
			}
		}
	}
}

// NotesSlide.notes_placeholder selects the first body placeholder in shape order.
// A missing placeholder remains distinct from an empty notes text frame.
func referenceSkillNotesBody(root *etree.Element) *etree.Element {
	tree := office.Child(office.Child(root, office.NSP, "cSld"), office.NSP, "spTree")
	for _, sp := range office.Children(tree, office.NSP, "sp") {
		props := office.Child(office.Child(sp, office.NSP, "nvSpPr"), office.NSP, "nvPr")
		ph := office.Child(props, office.NSP, "ph")
		if ph != nil && ph.SelectAttrValue("type", "obj") == "body" {
			return office.Child(sp, office.NSP, "txBody")
		}
	}
	return nil
}

func TestReferenceSkillNotesPlaceholderText(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`<a:p><a:r><a:t>Total: $200.</a:t></a:r></a:p>`, "Total: $200."},
		{`<a:p><a:r><a:t>A</a:t></a:r><a:br/><a:fld id="x"><a:t>B</a:t></a:fld></a:p><a:p/><a:p><a:r><a:t>C</a:t></a:r></a:p>`, "A\vB\n\nC"},
		{`<a:p/>`, ""},
	} {
		d := etree.NewDocument()
		raw := `<p:notes xmlns:p="` + office.NSP + `" xmlns:a="` + office.NSA + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="1" name="Footer"/><p:cNvSpPr/><p:nvPr><p:ph type="ftr"/></p:nvPr></p:nvSpPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Ignore footer</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes"/><p:cNvSpPr/><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody><a:bodyPr/><a:lstStyle/>` + tc.body + `</p:txBody></p:sp></p:spTree></p:cSld></p:notes>`
		if err := d.ReadFromString(raw); err != nil {
			t.Fatal(err)
		}
		body := referenceSkillNotesBody(d.Root())
		if body == nil || referenceRuntimeFrameText(body) != tc.want {
			t.Fatalf("notes: %q want %q", referenceRuntimeFrameText(body), tc.want)
		}
		tree := office.Child(office.Child(d.Root(), office.NSP, "cSld"), office.NSP, "spTree")
		tree.RemoveChild(tree.ChildElements()[1])
		if referenceSkillNotesBody(d.Root()) != nil {
			t.Fatal("footer treated as notes")
		}
	}
}

// referenceSkillCellText follows reference DOCX library _Cell.text -> CT_P.text -> CT_R.text.
// Nested tables, tracked revisions and field instructions are not paragraph text.
func referenceSkillCellText(cell *etree.Element) string {
	runText := func(run *etree.Element) string {
		var b strings.Builder
		for _, e := range run.ChildElements() {
			if e.NamespaceURI() != office.NSW {
				continue
			}
			switch e.Tag {
			case "t":
				b.WriteString(e.Text())
			case "tab", "ptab":
				b.WriteByte('\t')
			case "cr":
				b.WriteByte('\n')
			case "noBreakHyphen":
				b.WriteByte('-')
			case "br":
				kind := office.AttrV(e, office.NSW, "type")
				if kind == "" || kind == "textWrapping" {
					b.WriteByte('\n')
				}
			}
		}
		return b.String()
	}
	var paragraphs []string
	for _, p := range office.Children(cell, office.NSW, "p") {
		var b strings.Builder
		for _, e := range p.ChildElements() {
			if office.Is(e, office.NSW, "r") {
				b.WriteString(runText(e))
			}
			if office.Is(e, office.NSW, "hyperlink") {
				for _, r := range office.Children(e, office.NSW, "r") {
					b.WriteString(runText(r))
				}
			}
		}
		paragraphs = append(paragraphs, b.String())
	}
	return strings.Join(paragraphs, "\n")
}

func TestReferenceSkillCellText(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`<w:p><w:r><w:t>$200</w:t></w:r></w:p>`, "$200"},
		{`<w:p><w:r><w:t>A</w:t><w:tab/><w:ptab/><w:br/><w:cr/><w:noBreakHyphen/><w:br w:type="page"/><w:br w:type="column"/><w:instrText>ignored</w:instrText></w:r></w:p>`, "A\t\t\n\n-"},
		{`<w:p><w:hyperlink><w:r><w:t>link</w:t></w:r></w:hyperlink><w:del><w:r><w:t>deleted</w:t></w:r></w:del><w:ins><w:r><w:t>inserted</w:t></w:r></w:ins></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>nested</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p/>`, "link\n"},
	} {
		d := etree.NewDocument()
		if err := d.ReadFromString(`<w:tc xmlns:w="` + office.NSW + `">` + tc.body + `</w:tc>`); err != nil {
			t.Fatal(err)
		}
		if got := referenceSkillCellText(d.Root()); got != tc.want {
			t.Fatalf("%q != %q", got, tc.want)
		}
	}
}
