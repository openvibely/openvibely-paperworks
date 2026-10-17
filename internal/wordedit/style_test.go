package wordedit

import (
	"fmt"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// UNRUN: source model of Paragraph.style -> Styles.get_by_id(PARAGRAPH),
// CT_Styles.default_for, and BaseStyle.name in reference DOCX library v1.2.0.
// This predicate preserves the original p.style.name == 'Caption' assertion;
// it is not a general style-name or inheritance implementation.
func referenceParagraphIsCaption(p, styles *etree.Element) (bool, error) {
	id := office.AttrV(office.Child(office.Child(p, nsW, "pPr"), nsW, "pStyle"), nsW, "val")
	var selected, fallback *etree.Element
	for _, s := range office.Children(styles, nsW, "style") {
		// get_by_id selects the first ID match, even if it has the wrong type.
		if id != "" && selected == nil && office.AttrV(s, nsW, "styleId") == id {
			selected = s
		}
		if office.AttrV(s, nsW, "type") == "paragraph" {
			switch office.AttrV(s, nsW, "default") {
			case "1", "true", "on":
				fallback = s // last default in document order
			}
		}
	}
	if selected == nil || office.AttrV(selected, nsW, "type") != "paragraph" {
		selected = fallback
	}
	if selected == nil {
		return false, fmt.Errorf("paragraph has no resolved style (Python p.style.name would fail)")
	}
	name := office.AttrV(office.Child(selected, nsW, "name"), nsW, "val")
	// BabelFish maps internal 'caption' to UI 'Caption'; other aliases cannot
	// equal Caption. Missing name produces None in Reference, hence false here.
	return name == "caption" || name == "Caption", nil
}

func TestReferenceCaptionStyleContract(t *testing.T) {
	normal := `<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>`
	caption := `<w:style w:type="paragraph" w:styleId="CustomID"><w:name w:val="caption"/></w:style>`
	cases := []struct {
		name, id, styles string
		want, fail       bool
	}{
		{"name not ID", "CustomID", normal + caption, true, false},
		{"misleading ID", "Caption", normal + `<w:style w:type="paragraph" w:styleId="Caption"><w:name w:val="Other"/></w:style>`, false, false},
		{"missing ID defaults", "Missing", normal + caption, false, false},
		{"absent style defaults", "", normal + caption, false, false},
		{"last default", "", normal + `<w:style w:type="paragraph" w:default="true"><w:name w:val="Caption"/></w:style>`, true, false},
		{"wrong type defaults", "CustomID", normal + `<w:style w:type="character" w:styleId="CustomID"><w:name w:val="caption"/></w:style>`, false, false},
		{"first duplicate ID wins", "CustomID", normal + `<w:style w:type="character" w:styleId="CustomID"/>` + caption, false, false},
		{"absent type defaults", "CustomID", normal + `<w:style w:styleId="CustomID"><w:name w:val="caption"/></w:style>`, false, false},
		{"missing name", "CustomID", normal + `<w:style w:type="paragraph" w:styleId="CustomID"/>`, false, false},
		{"no default fails", "Missing", caption, false, true},
		{"false default ignored", "", `<w:style w:type="paragraph" w:default="false"><w:name w:val="Caption"/></w:style>`, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := etree.NewDocument()
			if err := d.ReadFromString(`<root xmlns:w="` + nsW + `"><w:p><w:pPr><w:pStyle w:val="` + c.id + `"/></w:pPr></w:p><w:styles>` + c.styles + `</w:styles></root>`); err != nil {
				t.Fatal(err)
			}
			got, err := referenceParagraphIsCaption(office.Child(d.Root(), nsW, "p"), office.Child(d.Root(), nsW, "styles"))
			referenceEqual(t, err != nil, c.fail)
			referenceEqual(t, got, c.want)
		})
	}
}
