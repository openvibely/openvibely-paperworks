package cli

import (
	"bytes"
	"encoding/json"
	"github.com/xuri/excelize/v2"
	"golang.org/x/image/webp"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestNewEditorCommandsAndSchemas(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "in.csv")
	os.WriteFile(csv, []byte("Name,Value\nExample,2\n"), 0600)
	in := filepath.Join(dir, "in.xlsx")
	runCLI(t, "import-csv", csv, "--output", in, "--sheet", "Data")
	spec := writeSpec(t, dir, "edit.json", map[string]any{"edits": []any{map[string]any{"operation": "formula", "sheet": "Data", "cell": "C2", "formula": "B2*2"}}})
	out := filepath.Join(dir, "out.xlsx")
	runCLI(t, "workbook-edit", in, "--spec", spec, "--output", out)
	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.GetCellFormula("Data", "C2")
	f.Close()
	if err != nil || v != "B2*2" {
		t.Fatal(v, err)
	}
	for _, cmd := range []string{"chart-series-replace", "create-pdf", "pdf-layout", "pdf-object-edit", "create-docx", "create-xlsx", "workbook-edit", "create-pptx", "slide-edit", "deck-edit"} {
		r := runCLI(t, "schema", cmd)
		if r["$defs"] == nil || r["$ref"] == nil {
			t.Fatal(cmd, r)
		}
	}

	word := runCLI(t, "schema", "word-edit")
	if word["properties"] == nil {
		t.Fatal("Word schema missing fields", word)
	}
	r := runCLI(t, "inspect-select", out, "--sheet", "Data", "--range", "A1:C2", "--search", "Example")
	if len(r["matches"].([]any)) == 0 {
		t.Fatal(r)
	}
}
func TestPreviewExportJPEGAndContactSheet(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpec(t, dir, "pdf.json", map[string]any{"canvas": map[string]any{"pages": []any{map[string]any{"width": 200, "height": 200, "elements": []any{map[string]any{"type": "text", "x": 20, "y": 40, "text": "Café"}}}}}})
	in := filepath.Join(dir, "in.pdf")
	runCLI(t, "create-pdf", "--spec", spec, "--output", in)
	r := runCLI(t, "preview-export", in, "--output", filepath.Join(dir, "preview"), "--format", "jpeg", "--columns", "1")
	pages := r["pages"].([]any)
	if len(pages) != 1 {
		t.Fatal(r)
	}
	data, err := os.ReadFile(pages[0].(string))
	if err != nil || !bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		t.Fatal("not JPEG", err)
	}
	contact, err := os.ReadFile(r["contact_sheet"].(string))
	if err != nil || !bytes.HasPrefix(contact, []byte("\x89PNG")) {
		t.Fatal("not PNG", err)
	}
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"pdf-layout", in}, &stdout, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	var rows []any
	if err = json.Unmarshal(stdout.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatal(stdout.String(), err)
	}
}

func TestPreviewWebPLossless(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preview.webp")
	im := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	im.SetNRGBA(2, 3, color.NRGBA{R: 37, G: 112, B: 200, A: 255})
	if err := writePreview(path, im, "webp"); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoded, err := webp.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != im.Bounds() || color.NRGBAModel.Convert(decoded.At(2, 3)) != im.NRGBAAt(2, 3) {
		t.Fatal("WebP did not preserve rendered pixels")
	}
}
