package wordedit

import (
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetedReplacementRetainsOtherOccurrences(t *testing.T) {
	src := mk(t, "targeted", `<w:p>`+bold("Pa")+run("ver Paver")+`</w:p><w:p>`+run("Paver")+`</w:p>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	_, err := Edit(src, out, map[string]any{"operation": "replace-text", "paragraph": float64(0), "old": "Paver", "new": "Acme", "occurrence": float64(0)}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := paras(t, out)
	if got[0] != "Acme Paver" || got[1] != "Paver" {
		t.Fatal(got)
	}
	a, _ := opc.Open(src)
	b, _ := opc.Open(out)
	for _, n := range a.Names() {
		if n != "word/document.xml" && string(a.Get(n)) != string(b.Get(n)) {
			t.Fatal("unrelated part changed", n)
		}
	}
}
func TestTargetedReplacementDoesNotTouchFields(t *testing.T) {
	src := mk(t, "field", `<w:p><w:fldSimple w:instr="DATE">`+run("Paver")+`</w:fldSimple></w:p>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	_, err := Edit(src, out, map[string]any{"operation": "replace-text", "paragraph": float64(0), "old": "Paver", "new": "Acme"}, "")
	if err == nil {
		t.Fatal("field replaced")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("partial output")
	}
}
func TestGeneralWordBatch(t *testing.T) {
	src := mk(t, "batch", `<w:p>`+run("Hello")+`</w:p>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	_, err := Edit(src, out, map[string]any{"operation": "batch", "edits": []any{map[string]any{"operation": "run-format", "paragraph": float64(0), "run": float64(0), "bold": true}, map[string]any{"operation": "paragraph-insert", "paragraph": float64(0), "position": "after", "text": "Second"}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := paras(t, out)
	if len(got) != 2 || got[1] != "Second" {
		t.Fatal(got)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("word/document.xml")
	if len(d.FindElements("//w:b")) != 1 {
		t.Fatal("missing bold")
	}
}

func TestInsertedTableRowIsBlank(t *testing.T) {
	src := mk(t, "row", `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid><w:tr><w:tc><w:p>`+run("Keep original")+`</w:p></w:tc></w:tr></w:tbl>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	_, err := Edit(src, out, map[string]any{"operation": "table-row-insert", "table": float64(0), "row": float64(0)}, "")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("word/document.xml")
	rows := d.FindElements("//w:tbl/w:tr")
	if len(rows) != 2 {
		t.Fatal("row count", len(rows))
	}
	if len(rows[0].FindElements(".//w:t")) != 0 {
		t.Fatal("new row copied old values")
	}
	if rows[1].FindElement(".//w:t").Text() != "Keep original" {
		t.Fatal("old row changed")
	}
}

func TestInsertedParagraphDoesNotDuplicateSectionBreak(t *testing.T) {
	src := mk(t, "section", `<w:p><w:pPr><w:sectPr><w:type w:val="continuous"/></w:sectPr></w:pPr>`+run("First")+`</w:p>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	_, err := Edit(src, out, map[string]any{"operation": "paragraph-insert", "paragraph": float64(0), "position": "after", "text": "New"}, "")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("word/document.xml")
	paras := d.FindElements("//w:body/w:p")
	if len(paras) != 2 || paras[1].FindElement("w:pPr/w:sectPr") != nil || paras[0].FindElement("w:pPr/w:sectPr") == nil {
		t.Fatal("section break copied or lost")
	}
}

func TestReplaceTextSkipsComplexFields(t *testing.T) {
	begin := `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> DOCPROPERTY Product </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r>`
	end := `<w:r><w:fldChar w:fldCharType="end"/></w:r>`
	for _, tc := range []struct {
		name, body string
		paragraph  int
		want       []string
	}{
		{"same paragraph", `<w:p>` + begin + run("Paver") + end + run(" Paver") + `</w:p>`, 0, []string{"Paver Acme"}},
		{"nested fields", `<w:p>` + begin + begin + run("Paver") + end + run(" Paver") + end + run(" Paver") + `</w:p>`, 0, []string{"Paver Paver Acme"}},
		{"spanning paragraphs", `<w:p>` + begin + run("Paver") + `</w:p><w:p>` + run("Paver") + end + run(" Paver") + `</w:p>`, 1, []string{"Paver", "Paver Acme"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := mk(t, "complex-field", tc.body, nil)
			out := filepath.Join(t.TempDir(), "out.docx")
			if _, err := Edit(src, out, map[string]any{"operation": "replace-text", "paragraph": float64(tc.paragraph), "old": "Paver", "new": "Acme"}, ""); err != nil {
				t.Fatal(err)
			}
			got := paras(t, out)
			if len(got) != len(tc.want) {
				t.Fatal(got)
			}
			for i, want := range tc.want {
				if got[i] != want {
					t.Fatalf("paragraph %d: %q, want %q", i, got[i], want)
				}
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("word/document.xml")
			if err != nil {
				t.Fatal(err)
			}
			for _, instr := range d.FindElements("//w:instrText") {
				if instr.Text() != " DOCPROPERTY Product " {
					t.Fatal("field instruction changed")
				}
			}
		})
	}
	src := mk(t, "field-only", `<w:p>`+begin+run("Paver")+end+`</w:p>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	if _, err := Edit(src, out, map[string]any{"operation": "replace-text", "paragraph": float64(0), "old": "Paver", "new": "Acme"}, ""); err == nil {
		t.Fatal("field-only replacement must fail")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("failed replacement published output", err)
	}
}

func TestExplicitRunFontOverridesTheme(t *testing.T) {
	src := mk(t, "theme-font", `<w:p><w:r><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:asciiTheme="minorHAnsi" w:hAnsiTheme="minorHAnsi" w:eastAsiaTheme="minorEastAsia" w:cstheme="minorBidi" w:hint="eastAsia"/></w:rPr><w:t>Keep text</w:t></w:r></w:p>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	if _, err := Edit(src, out, map[string]any{"operation": "run-format", "paragraph": float64(0), "run": float64(0), "font": "Courier New"}, ""); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	font := d.FindElement("//w:rFonts")
	for _, name := range []string{"ascii", "hAnsi", "eastAsia", "cs"} {
		if getW(font, name) != "Courier New" {
			t.Fatal("explicit font missing", name)
		}
	}
	for _, name := range []string{"asciiTheme", "hAnsiTheme", "eastAsiaTheme", "cstheme"} {
		if getW(font, name) != "" {
			t.Fatal("theme still overrides explicit font", name)
		}
	}
	if getW(font, "hint") != "eastAsia" || paras(t, out)[0] != "Keep text" {
		t.Fatal("unrelated content changed")
	}
}

func TestInsertedRowDoesNotDuplicateWrappedContent(t *testing.T) {
	src := mk(t, "wrapped-row", `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid><w:tr><w:tblPrEx><w:tblCellSpacing w:w="80" w:type="dxa"/></w:tblPrEx><w:trPr><w:trHeight w:val="400"/></w:trPr><w:tc><w:tcPr><w:tcW w:w="1000" w:type="dxa"/><w:shd w:fill="FF0000"/></w:tcPr><w:sdt><w:sdtPr><w:id w:val="7"/><w:tag w:val="customer"/></w:sdtPr><w:sdtContent><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:bookmarkStart w:id="8" w:name="Customer"/>`+run("Existing customer")+`<w:bookmarkEnd w:id="8"/></w:p></w:sdtContent></w:sdt><w:p/></w:tc></w:tr></w:tbl>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	if _, err := Edit(src, out, map[string]any{"operation": "table-row-insert", "table": float64(0), "row": float64(0)}, ""); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	rows := d.FindElements("//w:tbl/w:tr")
	if len(rows) != 2 {
		t.Fatal("wrong row count")
	}
	for _, tag := range []string{"t", "sdt", "bookmarkStart", "bookmarkEnd"} {
		if len(rows[0].FindElements(".//w:"+tag)) != 0 {
			t.Fatal("copied content or identifiers", tag)
		}
	}
	if rows[0].FindElement("w:tc/w:p") == nil {
		t.Fatal("cell has no paragraph")
	}
	if getW(rows[0].FindElement("w:tblPrEx/w:tblCellSpacing"), "w") != "80" {
		t.Fatal("lost row table properties")
	}
	if getW(rows[0].FindElement("w:trPr/w:trHeight"), "val") != "400" || getW(rows[0].FindElement("w:tc/w:tcPr/w:shd"), "fill") != "FF0000" {
		t.Fatal("lost row/cell formatting")
	}
	if rows[1].FindElement(".//w:t").Text() != "Existing customer" || len(d.FindElements("//w:sdtPr/w:id")) != 1 || len(d.FindElements("//w:bookmarkStart")) != 1 {
		t.Fatal("original content changed or identifiers duplicated")
	}
}

func TestInsertedRowKeepsGridWithWrappedAndNestedCells(t *testing.T) {
	src := mk(t, "wrapped-cells", `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="2000"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:w="1000" w:type="dxa"/></w:tcPr><w:tbl><w:tblGrid><w:gridCol w:w="500"/></w:tblGrid><w:tr><w:tc><w:p>`+run("Nested")+`</w:p></w:tc></w:tr></w:tbl><w:p/></w:tc><w:sdt><w:sdtPr><w:id w:val="9"/></w:sdtPr><w:sdtContent><w:tc><w:tcPr><w:tcW w:w="2000" w:type="dxa"/></w:tcPr><w:p>`+run("Wrapped")+`</w:p></w:tc></w:sdtContent></w:sdt></w:tr></w:tbl>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	if _, err := Edit(src, out, map[string]any{"operation": "table-row-insert", "table": float64(0), "row": float64(0)}, ""); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	rows := d.FindElements("/w:document/w:body/w:tbl/w:tr")
	if len(rows) != 2 {
		t.Fatal("wrong top-level row count", len(rows))
	}
	cells := rows[0].FindElements("w:tc")
	if len(cells) != 2 {
		t.Fatal("wrapped cell lost or nested cell added to grid", len(cells))
	}
	for i, width := range []string{"1000", "2000"} {
		if getW(cells[i].FindElement("w:tcPr/w:tcW"), "w") != width || cells[i].FindElement("w:p") == nil {
			t.Fatal("cell formatting or paragraph lost", i)
		}
	}
	if len(rows[0].FindElements(".//w:t"))+len(rows[0].FindElements(".//w:sdt"))+len(rows[0].FindElements(".//w:tbl")) != 0 {
		t.Fatal("content duplicated into blank row")
	}
	if len(rows[1].FindElements(".//w:t")) != 2 || len(rows[1].FindElements(".//w:sdt")) != 1 {
		t.Fatal("original row changed")
	}
}
