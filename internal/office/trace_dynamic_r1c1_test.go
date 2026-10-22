package office

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func TestINDIRECTRelativeR1C1TraceAndBounds(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", "Output"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Output", "E9", 8); err != nil {
		t.Fatal(err)
	}
	for cell, formula := range map[string]string{
		"F10": `INDIRECT("R[-1]C[-1]",FALSE)`,
		"F11": `INDIRECT("R[-99]C",FALSE)`,
	} {
		if err := f.SetCellFormula("Output", cell, formula); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "trace.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	report, err := TraceWorkbook(context.Background(), p, "Output", "F10", 200, 20)
	if err != nil || !report.Complete {
		t.Fatalf("trace failed: report=%+v err=%v", report, err)
	}
	if _, ok := report.Nodes["'Output'!E9"]; !ok {
		t.Fatalf("relative R1C1 target E9 missing: %+v", report.Nodes)
	}

	report, err = TraceWorkbook(context.Background(), p, "Output", "F11", 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || len(report.Unresolved) == 0 {
		t.Fatalf("out-of-bounds R1C1 target was not rejected: %+v", report)
	}
}

func TestR1C1AddressUsesSharedComponentResolution(t *testing.T) {
	for _, tc := range []struct {
		address string
		owner   string
		want    string
	}{
		{address: "R[-1]C[-1]", owner: "Output!F10", want: "E9"},
		{address: "'Input Data'!R[-1]C[-1]", owner: "Output!F10", want: "'Input Data'!E9"},
		{address: "RC", owner: "Output!F10", want: "F10"},
		{address: "R1C1", owner: "Output!F10", want: "A1"},
	} {
		got, err := r1c1Address(tc.address, tc.owner)
		if err != nil || got != tc.want {
			t.Errorf("r1c1Address(%q, %q) = %q, %v; want %q", tc.address, tc.owner, got, err, tc.want)
		}
	}
	for _, address := range []string{"R[-99]C", "R[1048567]C", "RC[-6]", "RC[16379]"} {
		if got, err := r1c1Address(address, "Output!F10"); err == nil {
			t.Errorf("r1c1Address(%q) accepted out-of-bounds target %q", address, got)
		}
	}
}
