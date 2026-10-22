package xlsx

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestRenameSheetControlLinks(t *testing.T) {
	for _, tc := range []struct{ name, source, target, host, link, want string }{
		{"checkbox", "A", "New A", "B", "A!$A$1", "'New A'!$A$1"},
		{"range", "Lists", "Choices", "B", "Lists!$A$1:$A$5", "'Choices'!$A$1:$A$5"},
		{"same sheet", "B", "New B", "B", "$C$3", "$C$3"},
		{"modern", "Data", "Inputs", "B", "Data!$B$2", "'Inputs'!$B$2"},
		{"case only", "Data", "DATA", "B", "Data!$B$2", "'DATA'!$B$2"},
		{"other sheet", "Data", "Inputs", "B", "B!$B$2", "B!$B$2"},
		{"external", "Data", "Inputs", "B", "[1]Data!A1", "[1]Data!A1"},
		{"literal", "Data", "Inputs", "B", `"Data!A1"`, `"Data!A1"`},
		{"entities", "A&B", "New & O'Data", "B", "'A&B'!$A$1", "'New & O''Data'!$A$1"},
	} {
		for _, form := range []string{"plain", "attributes", "cdata", "mixed", "comments", "mixed-comments"} {
			t.Run(tc.name+"/"+form, func(t *testing.T) {
				dir := t.TempDir()
				in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
				f := excelize.NewFile()
				defer f.Close()
				if err := f.SetSheetName("Sheet1", tc.source); err != nil {
					t.Fatal(err)
				}
				if tc.host != tc.source {
					if _, err := f.NewSheet(tc.host); err != nil {
						t.Fatal(err)
					}
				}
				for i, typ := range []excelize.FormControlType{excelize.FormControlCheckBox, excelize.FormControlScrollBar} {
					cell := []string{"D1", "D5"}[i]
					if err := f.AddFormControl(tc.host, excelize.FormControl{Cell: cell, Type: typ, CellLink: "A1"}); err != nil {
						t.Fatal(err)
					}
				}
				if err := f.SaveAs(in); err != nil {
					t.Fatal(err)
				}
				p, err := opc.Open(in)
				if err != nil {
					t.Fatal(err)
				}
				// Add list/dropdown range links to the real VML drawing, retaining
				// the existing checkbox and scrollbar links and relationships.
				var vmlName string
				for _, name := range p.Names() {
					if strings.HasSuffix(name, ".vml") {
						vmlName = name
						break
					}
				}
				if vmlName == "" {
					t.Fatal("missing control drawing")
				}
				var escaped bytes.Buffer
				if err := xml.EscapeText(&escaped, []byte(tc.link)); err != nil {
					t.Fatal(err)
				}
				vml := bytes.ReplaceAll(p.Get(vmlName), []byte("<x:FmlaLink>A1</x:FmlaLink>"), []byte("<x:FmlaLink>"+escaped.String()+"</x:FmlaLink>"))
				vml = bytes.ReplaceAll(vml, []byte("</x:ClientData>"), []byte("<x:FmlaRange>"+escaped.String()+"</x:FmlaRange></x:ClientData>"))
				if !bytes.Contains(vml, []byte("FmlaRange")) {
					t.Fatal("range fixture not inserted")
				}
				for _, tag := range []string{"FmlaLink", "FmlaRange"} {
					opening := "<x:" + tag + ">"
					text := escaped.String()
					switch form {
					case "attributes":
						opening = "<x:" + tag + ` xmlns:x="urn:schemas-microsoft-com:office:excel" >`
					case "cdata":
						text = "<![CDATA[" + tc.link + "]]>"
					case "mixed", "comments", "mixed-comments":
						// Split inside the sheet name to verify decoding happens
						// before the formula reference is rewritten.
						var suffix bytes.Buffer
						if err := xml.EscapeText(&suffix, []byte(tc.link[1:])); err != nil {
							t.Fatal(err)
						}
						text = "<![CDATA[" + tc.link[:1] + "]]>" + suffix.String()
						if form == "comments" {
							var first bytes.Buffer
							if err := xml.EscapeText(&first, []byte(tc.link[:1])); err != nil {
								t.Fatal(err)
							}
							text = "<!-- before -->" + first.String() + "<!-- inside -->" + suffix.String() + "<!-- after -->"
						}
						if form == "mixed-comments" {
							opening = "<x:" + tag + ` xmlns:x="urn:schemas-microsoft-com:office:excel">`
							text = "<!-- before --><![CDATA[" + tc.link[:1] + "]]><!-- inside -->" + suffix.String() + "<!-- after -->"
						}
					}
					vml = bytes.ReplaceAll(vml, []byte("<x:"+tag+">"+escaped.String()+"</x:"+tag+">"), []byte(opening+text+"</x:"+tag+">"))
				}
				p.Set(vmlName, vml)
				ctrlName := "xl/ctrlProps/ctrlProp1.xml"
				ctrl := []byte(`<c:formControlPr xmlns:c="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main" objectType="CheckBox" fmlaLink="` + escaped.String() + `" fmlaRange="` + escaped.String() + `" fmlaTxbx="` + escaped.String() + `"/>`)
				p.Set(ctrlName, ctrl)
				if err := p.EnsureContentType(ctrlName, "application/vnd.ms-excel.controlproperties+xml"); err != nil {
					t.Fatal(err)
				}
				// SaveNew intentionally rejects overwriting the original fixture.
				fixture := filepath.Join(dir, "fixture.xlsx")
				if err := p.SaveNew(fixture); err != nil {
					t.Fatal(err)
				}
				if err := EditFile(context.Background(), fixture, out, dir, EditSpec{Edits: []Edit{{Operation: "rename-sheet", Sheet: tc.source, Name: tc.target}}}); err != nil {
					t.Fatal(err)
				}
				result, err := opc.Open(out)
				if err != nil {
					t.Fatal(err)
				}
				for _, part := range []string{vmlName, ctrlName} {
					if tc.link == tc.want && !bytes.Equal(p.Get(part), result.Get(part)) {
						t.Fatalf("unchanged part %s was reserialized", part)
					}
				}
				if form == "comments" || form == "mixed-comments" {
					for _, comment := range []string{"<!-- before -->", "<!-- inside -->", "<!-- after -->"} {
						if bytes.Count(result.Get(vmlName), []byte(comment)) != bytes.Count(vml, []byte(comment)) {
							t.Fatalf("comment lost or changed: %s", comment)
						}
					}
				}
				d, err := result.XML(ctrlName)
				if err != nil {
					t.Fatal(err)
				}
				for _, attr := range []string{"fmlaLink", "fmlaRange", "fmlaTxbx"} {
					if got := d.Root().SelectAttrValue(attr, ""); got != tc.want {
						t.Fatalf("%s = %q, want %q", attr, got, tc.want)
					}
				}
				decoder := xml.NewDecoder(bytes.NewReader(result.Get(vmlName)))
				counts := map[string]int{}
				for {
					token, err := decoder.Token()
					if err != nil {
						if err == io.EOF {
							break
						}
						t.Fatal(err)
					}
					if start, ok := token.(xml.StartElement); ok && (start.Name.Local == "FmlaLink" || start.Name.Local == "FmlaRange") {
						var got string
						if err := decoder.DecodeElement(&got, &start); err != nil {
							t.Fatal(err)
						}
						if got != tc.want {
							t.Fatalf("%s = %q, want %q", start.Name.Local, got, tc.want)
						}
						counts[start.Name.Local]++
					}
				}
				if counts["FmlaLink"] != 2 || counts["FmlaRange"] != 2 {
					t.Fatalf("missing links: %v", counts)
				}
				reopened, err := excelize.OpenFile(out)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				if idx, err := reopened.GetSheetIndex(tc.target); err != nil || idx < 0 {
					t.Fatalf("renamed sheet missing: %v", err)
				}
			})
		}
	}

}

func TestRenameSheetPreservesCommentVML(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.xlsx"), filepath.Join(dir, "out.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	if err := f.AddComment("Sheet1", excelize.Comment{Cell: "A1", Author: "Tester", Text: "Sheet1!A1"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	before, err := opc.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{{Operation: "rename-sheet", Sheet: "Sheet1", Name: "Renamed"}}}); err != nil {
		t.Fatal(err)
	}
	after, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, name := range before.Names() {
		if strings.HasSuffix(name, ".vml") {
			count++
			if !bytes.Equal(before.Get(name), after.Get(name)) {
				t.Fatalf("comment drawing changed: %s", name)
			}
		}
	}
	if count == 0 {
		t.Fatal("missing comment drawing")
	}
}

func TestRenameVMLPreservesSurroundingBytes(t *testing.T) {
	input := `<xml><v:shape broken=unquoted><z:ClientData ObjectType="Drop"><z:FmlaLink >Data!$A$1</z:FmlaLink ><FmlaRange>Data!$A$1:$A$5</FmlaRange><z:FmlaTxbx>Data!B1</z:FmlaTxbx></z:ClientData></v:shape><!-- <x:FmlaLink>Data!A1</x:FmlaLink> --><![CDATA[<x:FmlaLink>Data!A1</x:FmlaLink>]]><x:FmlaLink>Other!A1</x:FmlaLink></xml>`
	want := strings.Replace(input, "Data!$A$1", "&#39;New Data&#39;!$A$1", 2)
	want = strings.Replace(want, "Data!B1", "&#39;New Data&#39;!B1", 1)
	if got := string(renameVMLControlFormulas([]byte(input), "Data", "New Data")); got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestRenameVMLListRanges(t *testing.T) {
	for _, kind := range []string{"Drop", "List"} {
		t.Run(kind, func(t *testing.T) {
			input := fmt.Sprintf(`<xml xmlns:x="urn:schemas-microsoft-com:office:excel"><x:ClientData ObjectType="%s"><x:FmlaRange>Lists!$A$1:$A$5</x:FmlaRange><x:FmlaLink>$C$3</x:FmlaLink></x:ClientData></xml>`, kind)
			want := fmt.Sprintf(`<xml xmlns:x="urn:schemas-microsoft-com:office:excel"><x:ClientData ObjectType="%s"><x:FmlaRange>&#39;Choices&#39;!$A$1:$A$5</x:FmlaRange><x:FmlaLink>$C$3</x:FmlaLink></x:ClientData></xml>`, kind)
			if got := string(renameVMLControlFormulas([]byte(input), "Lists", "Choices")); got != want {
				t.Fatalf("got %s; want %s", got, want)
			}
		})
	}
}

func TestRenameVMLAttributesAndCDATA(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{
			"local namespace and quoted attributes",
			`<z:FmlaLink xmlns:z='urn:schemas-microsoft-com:office:excel' data-note="a > b" >Data!$A$1</z:FmlaLink >`,
			`<z:FmlaLink xmlns:z='urn:schemas-microsoft-com:office:excel' data-note="a > b" >&#39;New Data&#39;!$A$1</z:FmlaLink >`,
		},
		{
			"CDATA range",
			`<x:FmlaRange><![CDATA[Data!$A$1:$A$5]]></x:FmlaRange>`,
			`<x:FmlaRange>&#39;New Data&#39;!$A$1:$A$5</x:FmlaRange>`,
		},
		{
			"mixed text and CDATA",
			`<FmlaLink xmlns="urn:schemas-microsoft-com:office:excel">D<![CDATA[at]]>a!$A$1</FmlaLink>`,
			`<FmlaLink xmlns="urn:schemas-microsoft-com:office:excel">&#39;New Data&#39;!$A$1</FmlaLink>`,
		},
		{
			"CDATA literal containing markup",
			`<x:FmlaLink><![CDATA[IF("<x:FmlaLink>Data!A1</x:FmlaLink>"="",Data!A1,0)]]></x:FmlaLink>`,
			`<x:FmlaLink>IF(&#34;&lt;x:FmlaLink&gt;Data!A1&lt;/x:FmlaLink&gt;&#34;=&#34;&#34;,&#39;New Data&#39;!A1,0)</x:FmlaLink>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := `<xml><v:shape broken=unquoted><!-- <x:FmlaLink xmlns:x="urn:schemas-microsoft-com:office:excel">Data!A1</x:FmlaLink> -->`
			suffix := `<![CDATA[<x:FmlaRange>Data!A1</x:FmlaRange>]]><x:FmlaLink xmlns:x="urn:schemas-microsoft-com:office:excel"><![CDATA[Other!A1]]></x:FmlaLink></v:shape></xml>`
			input := prefix + tc.input + suffix
			if got := string(renameVMLControlFormulas([]byte(input), "Data", "New Data")); got != prefix+tc.want+suffix {
				t.Fatalf("got %s; want %s", got, prefix+tc.want+suffix)
			}
			if got := string(renameVMLControlFormulas([]byte(input), "Absent", "New Data")); got != input {
				t.Fatal("drawing with no matching reference changed")
			}
		})
	}
}

func TestRenameVMLFormulaComments(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"leading", `<!-- linked cell -->Data!$A$1`, `<!-- linked cell -->&#39;New Data&#39;!$A$1`},
		{"trailing", `Data!$A$1<!-- linked cell -->`, `&#39;New Data&#39;!$A$1<!-- linked cell -->`},
		{"split qualifier", `Da<!-- split -->ta!$A$1`, `&#39;New Data&#39;<!-- split -->!$A$1`},
		{"entity and CDATA", `&#68;<!-- first --><![CDATA[at]]><!-- second -->a!$A$1`, `&#39;New Data&#39;<!-- first --><!-- second -->!$A$1`},
		{"quoted qualifier", `&apos;Da<!-- split -->ta&apos;!$A$1`, `&#39;New Data&#39;<!-- split -->!$A$1`},
		{"multiple references", `SUM(Da<!-- one -->ta!A1,Data<!-- two -->!A2)`, `SUM(&#39;New Data&#39;<!-- one -->!A1,&#39;New Data&#39;<!-- two -->!A2)`},
		{"comment markup", `<!-- <x:FmlaLink>Data!B2</x:FmlaLink> -->Data!A1`, `<!-- <x:FmlaLink>Data!B2</x:FmlaLink> -->&#39;New Data&#39;!A1`},
		{"unchanged chunks", `&quot;literal&quot;<!-- boundary -->&amp;Data!A1<!-- tail -->+&#49;`, `&quot;literal&quot;<!-- boundary -->&amp;&#39;New Data&#39;!A1<!-- tail -->+&#49;`},
		{"literal", `&quot;Da<!-- split -->ta!A1&quot;`, `&quot;Da<!-- split -->ta!A1&quot;`},
		{"external", `[1]Da<!-- split -->ta!A1`, `[1]Da<!-- split -->ta!A1`},
		{"same sheet", `<!-- link -->$C$3`, `<!-- link -->$C$3`},
		{"other sheet", `Other<!-- link -->!$C$3`, `Other<!-- link -->!$C$3`},
		{"processing instruction", `Da<?note keep?>ta!A1`, `&#39;New Data&#39;<?note keep?>!A1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, tag := range []string{"FmlaLink", "FmlaRange", "FmlaTxbx"} {
				prefix := `<xml><v:shape broken=unquoted><x:` + tag + ` xmlns:x='urn:schemas-microsoft-com:office:excel'>`
				suffix := `</x:` + tag + `><!-- Data!A1 --><?note <x:FmlaLink>Data!A1</x:FmlaLink>?><![CDATA[<x:FmlaLink>Data!A1</x:FmlaLink>]]></v:shape></xml>`
				input := prefix + tc.input + suffix
				if got := string(renameVMLControlFormulas([]byte(input), "Data", "New Data")); got != prefix+tc.want+suffix {
					t.Fatalf("%s: got %s; want %s", tag, got, prefix+tc.want+suffix)
				}
				if got := string(renameVMLControlFormulas([]byte(input), "Absent", "New Data")); got != input {
					t.Fatal("drawing without matching references changed")
				}
			}
		})
	}
}
