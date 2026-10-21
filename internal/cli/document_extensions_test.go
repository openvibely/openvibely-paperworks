package cli

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestDocumentExtensionsThroughCLI(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pptx")
	out := filepath.Join(dir, "out.pptx")
	spec := writeSpec(t, dir, "create.json", map[string]any{"slides": []any{map[string]any{"elements": []any{map[string]any{"type": "shape", "shape": "rect", "w": 3, "h": 2}, map[string]any{"type": "table", "x": 4, "w": 3, "h": 2, "rows": [][]string{{"A", "B"}, {"C", "D"}}}}}}})
	runCLI(t, "create-pptx", "--spec", spec, "--output", in)
	imagePath := filepath.Join(dir, "local.png")
	f, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{20, 120, 200, 255})
		}
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	edits := writeSpec(t, dir, "edit.json", map[string]any{"edits": []any{
		map[string]any{"operation": "drawing-style", "slide": 1, "shape_id": "2", "drawing_style": map[string]any{"image": map[string]any{"path": "local.png", "fit": "cover"}}},
		map[string]any{"operation": "table-merge", "slide": 1, "shape_id": "3", "end_row": 1, "end_column": 1},
		map[string]any{"operation": "notes", "slide": 1, "paragraphs": []any{map[string]any{"runs": []any{map[string]any{"text": "CLI rich note", "bold": true}}}}},
	}})
	runCLI(t, "slide-edit", in, "--spec", edits, "--output", out)
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := p.XML("ppt/slides/slide1.xml")
	if len(office.All(d.Root(), office.NSA, "blip")) != 1 {
		t.Fatal("relative image path not resolved")
	}
	n, _ := p.XML("ppt/notesSlides/notesSlide1.xml")
	if office.Text(n.Root(), office.NSA, "t") != "CLI rich note" {
		t.Fatal("notes not exposed")
	}
	r := runCLI(t, "render", out, "--output", filepath.Join(dir, "preview"))
	if r == nil {
		t.Fatal("missing rendering result")
	}
	for _, name := range []string{"create-pptx", "slide-edit"} {
		schema := runCLI(t, "schema", name)
		defs := schema["$defs"].(map[string]any)
		found := false
		for _, v := range defs {
			fields := v.(map[string]any)["properties"].(map[string]any)
			if fields["children"] != nil && fields["weight"] != nil {
				found = true
			}
		}
		if !found {
			t.Fatal("recursive layout schema missing", name)
		}
	}
	// Clone through JSON and resolve the external template relative to the spec.
	doc := filepath.Join(dir, "source.docx")
	create := writeSpec(t, dir, "word.json", map[string]any{"title": "Template", "blocks": []any{map[string]any{"type": "paragraph", "text": "Body"}}})
	runCLI(t, "create-docx", "--spec", create, "--output", doc)
	cloneSpec := writeSpec(t, dir, "clone.json", map[string]any{"operation": "clone-component", "path": "source.docx", "start": 0, "end": 0})
	clone := filepath.Join(dir, "clone.docx")
	runCLI(t, "word-edit", doc, "--spec", cloneSpec, "--output", clone)
	q, _ := opc.Open(clone)
	x, _ := q.XML("word/document.xml")
	if len(office.All(x.Root(), office.NSW, "p")) < 2 {
		t.Fatal("no cloned paragraph")
	}
	var stdout, stderr bytes.Buffer
	bad := writeSpec(t, dir, "bad.json", map[string]any{"edits": []any{map[string]any{"operation": "drawing-style", "slide": 1, "shape_id": "2", "drawing_style": map[string]any{"image": map[string]any{"path": ""}}}}})
	badOutput := filepath.Join(dir, "bad.pptx")
	if Main([]string{"slide-edit", in, "--spec", bad, "--output", badOutput}, &stdout, &stderr) == 0 {
		t.Fatal("empty image path accepted")
	}
	if _, err := os.Stat(badOutput); !os.IsNotExist(err) {
		t.Fatal("invalid edit published")
	}
}

// Exercise the renderer as well as the package: distinct style IDs alone do not
// prevent office applications from merging styles with the same display name.
func TestCloneComponentPreservesRenderedStyleColors(t *testing.T) {
	for _, sourceID := range []string{"Shared", "DifferentID"} {
		t.Run(sourceID, func(t *testing.T) {
			dir := t.TempDir()
			seed := filepath.Join(dir, "seed.docx")
			spec := writeSpec(t, dir, "seed.json", map[string]any{"blocks": []any{map[string]any{"type": "paragraph", "text": "Seed"}}})
			runCLI(t, "create-docx", "--spec", spec, "--output", seed)
			fixture := func(name, id, color string) string {
				p, err := opc.Open(seed)
				if err != nil {
					t.Fatal(err)
				}
				p.Set("word/document.xml", []byte(`<w:document xmlns:w="`+office.NSW+`"><w:body><w:p><w:pPr><w:pStyle w:val="`+id+`"/></w:pPr><w:r><w:t>`+name+`</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`))
				p.Set("word/styles.xml", []byte(`<w:styles xmlns:w="`+office.NSW+`"><w:style w:type="paragraph" w:styleId="`+id+`"><w:name w:val="Shared Style"/><w:rPr><w:color w:val="`+color+`"/></w:rPr></w:style></w:styles>`))
				path := filepath.Join(dir, name+".docx")
				if err := p.SaveNew(path); err != nil {
					t.Fatal(err)
				}
				return path
			}
			dest := fixture("Original", "Shared", "000000")
			src := fixture("Copied", sourceID, "FF0000")
			out := filepath.Join(dir, "out.docx")
			clone := writeSpec(t, dir, "clone.json", map[string]any{"operation": "clone-component", "path": src, "start": 0, "end": 0})
			runCLI(t, "word-edit", dest, "--spec", clone, "--output", out)
			runCLI(t, "render", out, "--output", filepath.Join(dir, "render"))
			f, err := os.Open(filepath.Join(dir, "render", "page-0001.png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			// Each line must retain its own color, in original-then-clone order.
			black, red, blackY, redY := 0, 0, 0, 0
			for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
				for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					if r < 0x6000 && g < 0x6000 && b < 0x6000 {
						black++
						blackY += y
					}
					if r > 0xA000 && g < 0x6000 && b < 0x6000 {
						red++
						redY += y
					}
				}
			}
			if black < 20 || red < 20 {
				t.Fatalf("lost text colors: black pixels=%d, red pixels=%d", black, red)
			}
			if blackY/black >= redY/red {
				t.Fatal("original and cloned text colors were swapped")
			}
		})
	}
}
