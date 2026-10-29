package wordedit

import (
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const gridBeforeTable = `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="2000"/><w:gridCol w:w="3000"/></w:tblGrid>
	<w:tr><w:trPr><w:gridBefore w:val="1"/></w:trPr>
	<w:tc><w:tcPr><w:shd w:fill="112233"/></w:tcPr><w:p><w:r><w:t>First</w:t></w:r></w:p></w:tc>
	<w:tc><w:tcPr><w:shd w:fill="445566"/></w:tcPr><w:p><w:r><w:t>Second</w:t></w:r></w:p></w:tc>
	</w:tr></w:tbl>`

func reopenEditedTable(t *testing.T, path string) *etree.Document {
	t.Helper()
	pkg, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := pkg.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func firstTableRow(t *testing.T, doc *etree.Document) (*etree.Element, []*etree.Element) {
	t.Helper()
	rows := doc.FindElements("//w:tbl/w:tr")
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	cells := rows[0].FindElements("w:tc")
	return rows[0], cells
}

func cellText(t *testing.T, cell *etree.Element) string {
	t.Helper()
	var text strings.Builder
	for _, node := range cell.FindElements(".//w:t") {
		text.WriteString(node.Text())
	}
	return text.String()
}

func TestTableGridColumnSelectsCellsAfterGridBeforeForTextAndFormat(t *testing.T) {
	src := mk(t, "grid-before-text-format", gridBeforeTable, nil)
	out := mustEdit(t, src, map[string]any{"operation": "batch", "edits": []any{
		map[string]any{"operation": "table-cell-text", "table": float64(0), "row": float64(0), "column": float64(1), "text": "Updated first"},
		map[string]any{"operation": "table-cell-format", "table": float64(0), "row": float64(0), "column": float64(2), "fill": "AABBCC", "bold": true},
	}})

	row, cells := firstTableRow(t, reopenEditedTable(t, out))
	if len(cells) != 2 {
		t.Fatalf("physical cell count = %d, want 2", len(cells))
	}
	if getW(row.FindElement("w:trPr/w:gridBefore"), "val") != "1" {
		t.Fatal("gridBefore changed during cell edits")
	}
	if got := cellText(t, cells[0]); got != "Updated first" {
		t.Fatalf("grid column 1 text = %q, want Updated first", got)
	}
	if got := cellText(t, cells[1]); got != "Second" {
		t.Fatalf("grid column 2 text = %q, want Second", got)
	}
	if got := getW(cells[0].FindElement("w:tcPr/w:shd"), "fill"); got != "112233" {
		t.Fatalf("first cell fill = %q, want 112233", got)
	}
	if got := getW(cells[1].FindElement("w:tcPr/w:shd"), "fill"); got != "AABBCC" {
		t.Fatalf("second cell fill = %q, want AABBCC", got)
	}
	if bold := cells[1].FindElement("w:p/w:r/w:rPr/w:b"); bold == nil || getW(bold, "val") != "1" {
		t.Fatal("formatting did not apply to the cell at grid column 2")
	}
}

func TestTableGridColumnOmittedByGridBeforeErrorsWithoutEditingCells(t *testing.T) {
	src := mk(t, "grid-before-omitted-column", gridBeforeTable, nil)
	_, err := edit(t, src, map[string]any{"operation": "table-cell-text", "table": float64(0), "row": float64(0), "column": float64(0), "text": "Must not replace either cell"})
	if err == nil || !strings.Contains(err.Error(), "omitted") {
		t.Fatalf("column 0 error = %v, want a clear omitted-column error", err)
	}
	row, cells := firstTableRow(t, reopenEditedTable(t, src))
	if getW(row.FindElement("w:trPr/w:gridBefore"), "val") != "1" || len(cells) != 2 {
		t.Fatal("failed edit changed row grid properties or cell count")
	}
	if got := cellText(t, cells[0]); got != "First" {
		t.Fatalf("first physical cell changed to %q", got)
	}
	if got := cellText(t, cells[1]); got != "Second" {
		t.Fatalf("second physical cell changed to %q", got)
	}
}

func TestTableGridColumnInsideGridSpanResolvesToSpanningCell(t *testing.T) {
	body := `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="2000"/><w:gridCol w:w="3000"/></w:tblGrid>
		<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/></w:tcPr><w:p><w:r><w:t>Spanning</w:t></w:r></w:p></w:tc>
		<w:tc><w:p><w:r><w:t>Following</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	src := mk(t, "grid-span-selection", body, nil)
	out := mustEdit(t, src, map[string]any{"operation": "table-cell-text", "table": float64(0), "row": float64(0), "column": float64(1), "text": "Updated spanning cell"})
	_, cells := firstTableRow(t, reopenEditedTable(t, out))
	if len(cells) != 2 || getW(cells[0].FindElement("w:tcPr/w:gridSpan"), "val") != "2" {
		t.Fatal("edit changed the spanning cell structure")
	}
	if got := cellText(t, cells[0]); got != "Updated spanning cell" {
		t.Fatalf("grid column inside span selected wrong cell: %q", got)
	}
	if got := cellText(t, cells[1]); got != "Following" {
		t.Fatalf("edit changed the neighboring cell to %q", got)
	}
}

func TestTableMergeAndUnmergeUseGridColumnsAfterGridBefore(t *testing.T) {
	src := mk(t, "grid-before-merge", gridBeforeTable, nil)
	mergedPath := mustEdit(t, src, map[string]any{"operation": "table-merge", "table": float64(0), "row": float64(0), "column": float64(1), "end_column": float64(2)})
	mergedRow, mergedCells := firstTableRow(t, reopenEditedTable(t, mergedPath))
	if len(mergedCells) != 1 || getW(mergedRow.FindElement("w:trPr/w:gridBefore"), "val") != "1" {
		t.Fatal("merge selected the wrong cells or changed gridBefore")
	}
	if got := getW(mergedCells[0].FindElement("w:tcPr/w:gridSpan"), "val"); got != "2" {
		t.Fatalf("merged gridSpan = %q, want 2", got)
	}
	if got := cellText(t, mergedCells[0]); got != "FirstSecond" {
		t.Fatalf("merged content = %q, want FirstSecond", got)
	}

	unmergedPath := mustEdit(t, mergedPath, map[string]any{"operation": "table-unmerge", "table": float64(0), "row": float64(0), "column": float64(2)})
	unmergedRow, unmergedCells := firstTableRow(t, reopenEditedTable(t, unmergedPath))
	if len(unmergedCells) != 2 || getW(unmergedRow.FindElement("w:trPr/w:gridBefore"), "val") != "1" {
		t.Fatal("unmerge failed to resolve a column inside the spanning cell")
	}
	if unmergedCells[0].FindElement("w:tcPr/w:gridSpan") != nil {
		t.Fatal("unmerge left gridSpan on the original cell")
	}
	if got := cellText(t, unmergedCells[0]); got != "FirstSecond" {
		t.Fatalf("unmerge changed merged content to %q", got)
	}
}

func TestTableColumnStructureEditRejectsGridBeforeRows(t *testing.T) {
	src := mk(t, "grid-before-structure-restriction", gridBeforeTable, nil)
	_, err := edit(t, src, map[string]any{"operation": "table-column-insert", "table": float64(0), "column": float64(1)})
	if err == nil || !strings.Contains(err.Error(), "unmerged rectangular table") {
		t.Fatalf("column insert error = %v, want rectangular-table restriction", err)
	}
	row, cells := firstTableRow(t, reopenEditedTable(t, src))
	if getW(row.FindElement("w:trPr/w:gridBefore"), "val") != "1" || len(cells) != 2 {
		t.Fatal("rejected structural edit changed the source row")
	}
}
