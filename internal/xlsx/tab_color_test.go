package xlsx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestEditTabColor(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.xlsx")
	f := excelize.NewFile()
	theme, tint := 4, 0.5
	if err := f.SetSheetProps("Sheet1", &excelize.SheetPropsOptions{TabColorTheme: &theme, TabColorTint: &tint}); err != nil {
		t.Fatal(err)
	}
	f.SetCellValue("Sheet1", "A1", 42)
	f.SetCellFormula("Sheet1", "B1", "A1*2")
	if err := f.SaveAs(in); err != nil {
		t.Fatal(err)
	}
	f.Close()
	red := "ff0000"
	out := filepath.Join(dir, "red.xlsx")
	if err := EditFile(context.Background(), in, out, dir, EditSpec{Edits: []Edit{
		{Operation: "rename-sheet", Sheet: "Sheet1", Name: "Result"},
		{Operation: "tab-color", Sheet: "Result", Color: &red},
	}}); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	w, err := office.OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	props := office.Child(w.Sheets["Result"].Doc.Root(), office.NSS, "sheetPr")
	c := office.Child(props, office.NSS, "tabColor")
	if c == nil || c.SelectAttrValue("rgb", "") != "FFFF0000" || len(c.Attr) != 1 {
		t.Fatalf("unexpected tab color: %v", c)
	}
	g, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := g.GetCellValue("Result", "A1")
	formula, _ := g.GetCellFormula("Result", "B1")
	g.Close()
	if v != "42" || formula != "A1*2" {
		t.Fatalf("cells changed: %s %s", v, formula)
	}
	empty := ""
	cleared := filepath.Join(dir, "clear.xlsx")
	if err := EditFile(context.Background(), out, cleared, dir, EditSpec{Edits: []Edit{{Operation: "tab-color", Sheet: "Result", Color: &empty}}}); err != nil {
		t.Fatal(err)
	}
	p, _ = opc.Open(cleared)
	w, _ = office.OpenWorkbook(p)
	if len(office.All(w.Sheets["Result"].Doc.Root(), office.NSS, "tabColor")) != 0 {
		t.Fatal("tab color not cleared")
	}
	for _, value := range []string{"123", "GG0000", "#ff0000"} {
		if err := EditFile(context.Background(), in, filepath.Join(dir, "invalid.xlsx"), dir, EditSpec{Edits: []Edit{{Operation: "tab-color", Sheet: "Sheet1", Color: &value}}}); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
