package pdf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"os"
	"path/filepath"
	"testing"
)

func TestReportLabFixedParagraphs(t *testing.T) {
	raw, err := os.ReadFile("testdata/reportlab-paragraphs.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Cases []struct {
			Name         string
			Paragraph    FixedParagraph
			FontResource string `json:"font_resource"`
			Stream       string
		}
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	for _, c := range data.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if err := c.Paragraph.validate(context.Background()); err != nil {
				t.Fatal(err)
			}
			var b bytes.Buffer
			if err := c.Paragraph.paint(context.Background(), &b, c.FontResource); err != nil {
				t.Fatal(err)
			}
			if b.String() != c.Stream {
				t.Fatalf("got %q want %q", b.String(), c.Stream)
			}
		})
	}
}
func TestFixedParagraphRejectsUnsupportedLayout(t *testing.T) {
	p := FixedParagraph{Text: "hello world", Font: "Helvetica", FontSize: 12, Leading: 14, Width: 200}
	for _, mutate := range []func(*FixedParagraph){func(p *FixedParagraph) { p.Width = 1 }, func(p *FixedParagraph) { p.Text = "two\nlines" }, func(p *FixedParagraph) { p.Align = "justify" }, func(p *FixedParagraph) { p.Text = "two  spaces" }, func(p *FixedParagraph) { p.Font = "custom" }} {
		q := p
		mutate(&q)
		if q.validate(context.Background()) == nil {
			t.Fatalf("accepted unsupported %+v", q)
		}
	}
}

func TestInstalledReportPageStream(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/document-examples/document_pdf/examples/report.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec CreateSpec
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "report.pdf")
	ctx := context.Background()
	if err = Create(ctx, spec, "", out); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	pd, _, err := pageDict(ctx, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	sd, _ := streamOf(c.XRefTable, pd["Contents"])
	if sd == nil {
		t.Fatal("expected single complete stream")
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(sd.Raw)); got != "c84ac6f8ada01a7fcb86acabdffcb738278e4f210c887450fa73b2269b0cf2e9" {
		t.Fatalf("encoded stream differs: %s", got)
	}
	res := dictOf(c.XRefTable, pd["Resources"]).d
	if _, ok := res["Font"].(types.IndirectRef); !ok {
		t.Fatal("font dictionary must be indirect")
	}
	fonts := dictOf(c.XRefTable, res["Font"]).d
	for _, key := range []string{"F1", "F2"} {
		if _, ok := fonts[key].(types.IndirectRef); !ok {
			t.Fatal("font must be indirect")
		}
		if dictOf(c.XRefTable, fonts[key]).d["Name"] != types.Name(key) {
			t.Fatal("font name missing")
		}
	}
	if pd["CropBox"] != nil || pd["Rotate"] != types.Integer(0) {
		t.Fatal("page dictionary differs")
	}
	if err = sd.Decode(); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(sd.Content)); got != "99a80e6ad58a6c57793b60f33c110c4dbe9e41d3c27dfbd9c79aaa914a211557" {
		t.Fatalf("complete reference stream differs: %s", got)
	}
}
