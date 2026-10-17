package pdf

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedReportTableStreams(t *testing.T) {
	raw, err := os.ReadFile("testdata/reportlab-tables.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Name   string
			Table  FixedTable
			Stream string
		}
		Numbers []struct {
			Input    float64
			Expected string
		}
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if err := c.Table.validate(); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			c.Table.paint(&out, "F1")
			if c.Name == "hairline" {
				// ReportLab omits zero width and inherits the canvas width.
				// Our explicit hairline contract remains; do not claim equivalence.
				if out.String() == c.Stream || !bytes.Contains(out.Bytes(), []byte("0 w\n")) {
					t.Fatal("hairline boundary lost")
				}
				return
			}
			if out.String() != c.Stream {
				t.Fatalf("content stream differs\ngot:\n%s\nwant:\n%s", out.String(), c.Stream)
			}
		})
	}
	for _, n := range f.Numbers {
		if got := reportNumber(n.Input); got != n.Expected {
			t.Errorf("%v: %q want %q", n.Input, got, n.Expected)
		}
	}
}

func TestReportReusesHelvetica(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/document-examples/document_pdf/examples/report.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec CreateSpec
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "report.pdf")
	ctx := context.Background()
	if err = Create(ctx, spec, "", output); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	pd, inh, err := pageDict(ctx, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := writableResources(c.XRefTable, pd, inh)
	if err != nil {
		t.Fatal(err)
	}
	fonts := dictOf(c.XRefTable, res["Font"]).d
	if len(fonts) != 2 {
		t.Fatalf("duplicate font resource: %v", fonts)
	}
}
