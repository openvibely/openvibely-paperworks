package xlsx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Captured from the local pinned openpyxl writer, without XML normalization.
func TestColumnChartReferenceBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/chart-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Title, Sheet string
		Parts              map[string]string
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			q := "'" + strings.ReplaceAll(c.Sheet, "'", "''") + "'!"
			chart := ColumnChart{Cell: "F2", Title: c.Title, ValueTitle: "USD", WidthEMU: 5400000, HeightEMU: 2700000, Series: []ColumnSeries{{Name: q + "D1", Categories: q + "$A$2:$A$3", Values: q + "$D$2:$D$3"}}}
			out := filepath.Join(t.TempDir(), "new.xlsx")
			if err := Create(context.Background(), Spec{Sheets: []Sheet{{Name: c.Sheet, ColumnCharts: []ColumnChart{chart}}}}, "", out); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			for name, want := range c.Parts {
				native := name
				if got := fmt.Sprintf("%x", sha256.Sum256(p.Get(native))); got != want {
					t.Errorf("%s: %s want %s", name, got, want)
				}
			}
		})
	}
}
