package wordedit

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestRepeatedEditsAllocateConsecutivePartNames(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "pic.png"))
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	f.Close()
	src := mk(t, "unused-name", `<w:p>`+run("Body")+`</w:p>`, nil)
	out := filepath.Join(dir, "out.docx")
	insert := map[string]any{"operation": "image-insert", "paragraph": float64(0), "path": "pic.png", "width_in": float64(1)}
	header := map[string]any{"operation": "header-footer", "section": 0.0, "kind": "header", "text": "Head"}
	footer := map[string]any{"operation": "header-footer", "section": 0.0, "kind": "footer", "text": "Foot"}
	if _, err = Edit(src, out, map[string]any{"operation": "batch", "edits": []any{insert, insert, header, header, footer}}, dir); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"word/media/edit1.png", "word/media/edit2.png", "word/headerEdit1.xml", "word/headerEdit2.xml", "word/footerEdit1.xml"} {
		if !p.Has(name) {
			t.Error("missing", name)
		}
	}
	if p.Has("word/media/edit3.png") || p.Has("word/footerEdit2.xml") {
		t.Error("unexpected extra part")
	}
}
