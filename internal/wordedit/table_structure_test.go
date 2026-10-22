package wordedit

import (
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func editedTableStructureDoc(t *testing.T, body string, spec map[string]any) *etree.Document {
	t.Helper()
	src := mk(t, "table-structure", body, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	if _, err := Edit(src, out, spec, ""); err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestTableColumnInsertCreatesBlankCellsWithIndependentProperties(t *testing.T) {
	doc := editedTableStructureDoc(t, `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="2000"/></w:tblGrid>
		<w:tr><w:tc><w:p/></w:tc><w:tc><w:tcPr><w:tcW w:w="2400" w:type="dxa"/><w:shd w:fill="AABBCC"/><w:tcBorders><w:top w:val="single" w:sz="8" w:color="112233"/></w:tcBorders></w:tcPr><w:p><w:r><w:t>Formatted donor</w:t></w:r></w:p><w:altChunk r:id="rId42"/></w:tc></w:tr>
		<w:tr><w:tc><w:p/></w:tc><w:tc><w:p><w:r><w:t>Plain donor</w:t></w:r></w:p><w:altChunk r:id="rId43"/></w:tc></w:tr>
	</w:tbl>`, map[string]any{"operation": "table-column-insert", "table": float64(0), "column": float64(1)})

	rows := doc.FindElements("//w:tbl/w:tr")
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(rows))
	}
	for rowIndex, row := range rows {
		cells := row.FindElements("w:tc")
		if len(cells) != 3 {
			t.Fatalf("row %d cell count = %d, want 3", rowIndex, len(cells))
		}
		inserted := cells[1]
		paras := inserted.FindElements("w:p")
		if len(paras) != 1 || len(paras[0].ChildElements()) != 0 {
			t.Fatalf("row %d inserted cell does not have exactly one empty paragraph", rowIndex)
		}
		if len(inserted.FindElements(".//w:t")) != 0 || inserted.FindElement("w:altChunk") != nil {
			t.Fatalf("row %d inserted cell copied source content", rowIndex)
		}
		if rowIndex == 0 {
			if inserted.FindElement("w:tcPr") == nil || len(inserted.ChildElements()) != 2 {
				t.Fatal("formatted inserted cell should contain only copied properties and one paragraph")
			}
			if getW(inserted.FindElement("w:tcPr/w:tcW"), "w") != "2400" ||
				getW(inserted.FindElement("w:tcPr/shd"), "fill") != "AABBCC" ||
				getW(inserted.FindElement("w:tcPr/tcBorders/top"), "color") != "112233" {
				t.Fatal("inserted cell did not retain width, shading, and borders")
			}
		} else {
			if inserted.FindElement("w:tcPr") != nil || len(inserted.ChildElements()) != 1 {
				t.Fatal("cell without properties should contain only one paragraph")
			}
		}
	}

	inserted := rows[0].FindElements("w:tc")[1]
	donor := rows[0].FindElements("w:tc")[2]
	setW(inserted.FindElement("w:tcPr/tcW"), "w", "3000")
	setW(inserted.FindElement("w:tcPr/shd"), "fill", "DDEEFF")
	setW(inserted.FindElement("w:tcPr/tcBorders/top"), "color", "445566")
	if getW(donor.FindElement("w:tcPr/tcW"), "w") != "2400" ||
		getW(donor.FindElement("w:tcPr/shd"), "fill") != "AABBCC" ||
		getW(donor.FindElement("w:tcPr/tcBorders/top"), "color") != "112233" {
		t.Fatal("mutating copied cell properties changed the donor")
	}
	setW(donor.FindElement("w:tcPr/tcW"), "w", "4000")
	setW(donor.FindElement("w:tcPr/shd"), "fill", "FFFFFF")
	setW(donor.FindElement("w:tcPr/tcBorders/top"), "color", "778899")
	if getW(inserted.FindElement("w:tcPr/tcW"), "w") != "3000" ||
		getW(inserted.FindElement("w:tcPr/shd"), "fill") != "DDEEFF" ||
		getW(inserted.FindElement("w:tcPr/tcBorders/top"), "color") != "445566" {
		t.Fatal("mutating donor properties changed the copied cell")
	}
}

func TestTableUnmergeCreatesBlankSiblingsWithoutGridSpan(t *testing.T) {
	doc := editedTableStructureDoc(t, `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="2000"/><w:gridCol w:w="3000"/></w:tblGrid>
		<w:tr><w:tc><w:tcPr><w:tcW w:w="3000" w:type="dxa"/><w:gridSpan w:val="2"/><w:shd w:fill="AABBCC"/><w:tcBorders><w:bottom w:val="single" w:sz="6" w:color="112233"/></w:tcBorders></w:tcPr><w:p><w:r><w:t>Merged content stays</w:t></w:r></w:p><w:altChunk r:id="rId44"/></w:tc><w:tc><w:p><w:r><w:t>Following cell</w:t></w:r></w:p></w:tc></w:tr>
	</w:tbl>`, map[string]any{"operation": "table-unmerge", "table": float64(0), "row": float64(0), "column": float64(0)})

	rows := doc.FindElements("//w:tbl/w:tr")
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	cells := rows[0].FindElements("w:tc")
	if len(cells) != 3 {
		t.Fatalf("cell count = %d, want 3", len(cells))
	}
	original, sibling, following := cells[0], cells[1], cells[2]
	if original.FindElement("w:tcPr/gridSpan") != nil {
		t.Fatal("unmerge left gridSpan on the original cell")
	}
	if original.FindElement(".//w:t").Text() != "Merged content stays" || original.FindElement("w:altChunk") == nil {
		t.Fatal("unmerge moved or removed original cell content")
	}
	if following.FindElement(".//w:t").Text() != "Following cell" {
		t.Fatal("unmerge changed the following cell position or content")
	}
	if sibling.FindElement("w:tcPr/gridSpan") != nil ||
		getW(sibling.FindElement("w:tcPr/tcW"), "w") != "3000" ||
		getW(sibling.FindElement("w:tcPr/shd"), "fill") != "AABBCC" ||
		getW(sibling.FindElement("w:tcPr/tcBorders/bottom"), "color") != "112233" {
		t.Fatal("unmerged sibling did not receive the remaining cell properties")
	}
	if len(sibling.ChildElements()) != 2 || len(sibling.FindElements("w:p")) != 1 ||
		len(sibling.FindElements("w:p")[0].ChildElements()) != 0 ||
		len(sibling.FindElements(".//w:t")) != 0 || sibling.FindElement("w:altChunk") != nil {
		t.Fatal("unmerged sibling should contain copied properties and one empty paragraph only")
	}
	if len(doc.FindElements("//w:tbl/w:tblGrid/w:gridCol")) != 3 {
		t.Fatal("unmerge changed the table grid")
	}
}
