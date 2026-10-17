package pdf

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/primitives"
)

func sampleFixedTable() FixedTable {
	return FixedTable{Position: [2]float64{20.25, 30.75}, ColumnWidths: []float64{90.5, 40}, RowHeight: 20.25, FontSize: 10, PaddingLeft: 6, BaselineOffset: 5, Header: []string{"Label", "Value"}, Rows: [][]string{{`a(b)\c`, "12"}}, HeaderColor: [3]float64{.1, .2, .3}, GridColor: [3]float64{.8, .8, .8}, GridWidth: .5}
}

func TestFixedTablePrecisionAndText(t *testing.T) {
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "table.pdf")
	spec := CreateSpec{Pages: []CreatePage{{Size: &[2]float64{300.125, 400.625}, Text: []*primitives.TextBox{{Value: "Existing text", Position: [2]float64{20, 200}, Font: &primitives.FormFont{Name: "Helvetica-Bold", Size: 12}}}, FixedTables: []FixedTable{sampleFixedTable()}}}}
	if e := Create(ctx, spec, "", out); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	c, e := Load(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	_, inh, e := pageDict(ctx, c, 1)
	if e != nil {
		t.Fatal(e)
	}
	if inh.MediaBox.Width() != 300.125 || inh.MediaBox.Height() != 400.625 {
		t.Fatal(inh.MediaBox)
	}
	text, e := PageText(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"Existing text", "Label", "Value", `a(b)\c`, "12"} {
		if !strings.Contains(strings.Join(text, "\n"), s) {
			t.Fatalf("lost text %q in %v", s, text)
		}
	}
	report, e := Report(ctx, out)
	if e != nil || len(report.Issues) != 0 {
		t.Fatalf("report: %+v %v", report, e)
	}
}

func TestFixedTableRejectsInvalidBeforePublication(t *testing.T) {
	for _, name := range []string{"nan-size", "negative-size", "zero-width", "nan-color", "negative-stroke", "ragged", "header-width", "multiline", "unicode", "overflow"} {
		t.Run(name, func(t *testing.T) {
			table := sampleFixedTable()
			size := [2]float64{300, 400}
			switch name {
			case "nan-size":
				size[0] = math.NaN()
			case "negative-size":
				size[1] = -1
			case "zero-width":
				table.ColumnWidths[0] = 0
			case "nan-color":
				table.HeaderColor[0] = math.NaN()
			case "negative-stroke":
				table.GridWidth = -.5
			case "ragged":
				table.Rows[0] = []string{"only"}
			case "header-width":
				table.Header = []string{"only"}
			case "multiline":
				table.Rows[0][0] = "a\nb"
			case "unicode":
				table.Rows[0][0] = "é"
			case "overflow":
				table.RowHeight = math.MaxFloat64
			}
			out := filepath.Join(t.TempDir(), "bad.pdf")
			e := Create(context.Background(), CreateSpec{Pages: []CreatePage{{Size: &size, FixedTables: []FixedTable{table}}}}, "", out)
			if e == nil {
				t.Fatal("accepted invalid table/page")
			}
			if _, e = os.Stat(out); !os.IsNotExist(e) {
				t.Fatal("published invalid document", e)
			}
		})
	}
}

func TestFixedTablePreservesFormAndOtherPage(t *testing.T) {
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "mixed.pdf")
	spec := CreateSpec{Paper: "Letter", Pages: []CreatePage{{Size: &[2]float64{500.5, 600.25}, FixedTables: []FixedTable{sampleFixedTable()}, TextFields: []*primitives.TextField{{ID: "reviewer", Value: "Draft", Position: [2]float64{20, 200}, Width: 100, Height: 20}}}, {FixedTables: []FixedTable{sampleFixedTable()}}}}
	if e := Create(ctx, spec, "", out); e != nil {
		t.Fatal(e)
	}
	r, e := Report(ctx, out)
	if e != nil || r.Pages != 2 || len(r.Issues) != 0 || r.Fields["reviewer"] == nil || *r.Fields["reviewer"] != "Draft" {
		t.Fatalf("%+v %v", r, e)
	}
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	c, e := Load(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	_, inh, e := pageDict(ctx, c, 2)
	if e != nil {
		t.Fatal(e)
	}
	if inh.MediaBox.Width() != 612 || inh.MediaBox.Height() != 792 {
		t.Fatal("size leaked to second page", inh.MediaBox)
	}
}
