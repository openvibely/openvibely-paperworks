package wordedit

import (
	"bytes"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceLinkedImageRemovesExternalRelationship(t *testing.T) {
	dir := t.TempDir()
	makePNG := func(t *testing.T, name string, c color.Color) (string, []byte) {
		t.Helper()
		im := image.NewRGBA(image.Rect(0, 0, 10, 10))
		for y := 0; y < 10; y++ {
			for x := 0; x < 10; x++ {
				im.Set(x, y, c)
			}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, im); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, data.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		return file, data.Bytes()
	}
	_, originalData := makePNG(t, "original.png", color.RGBA{R: 255, A: 255})
	replacementPath, replacementData := makePNG(t, "replacement.png", color.RGBA{B: 255, A: 255})
	body := `<w:p><w:r><w:drawing><wp:inline xmlns:a="` + nsA + `" xmlns:pic="` + nsPic + `"><a:graphic><a:graphicData uri="` + nsPic + `"><pic:pic><pic:blipFill><a:blip r:embed="old-embed" r:link="old-link"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`
	src := mk(t, "linked-image", body, nil)
	var oldEmbedID, oldLinkID string
	src = referenceRewrite(t, src, func(p *opc.Package) {
		p.Set("word/media/original.png", originalData)
		if err := p.EnsureContentType(".png", "image/png"); err != nil {
			t.Fatal(err)
		}
		embedID, err := p.AddRel("word/document.xml", relBase+"image", "media/original.png", false)
		if err != nil {
			t.Fatal(err)
		}
		oldEmbedID = embedID
		linkID, err := p.AddRel("word/document.xml", relBase+"image", "https://example.test/original.png", true)
		if err != nil {
			t.Fatal(err)
		}
		oldLinkID = linkID
		referenceSetXML(t, p, "word/document.xml", func(root *etree.Element) {
			blips := office.All(root, nsA, "blip")
			if len(blips) != 1 {
				t.Fatalf("got %d blips, want 1", len(blips))
			}
			blips[0].CreateAttr("r:embed", embedID)
			blips[0].CreateAttr("r:link", linkID)
		})
	})

	out := filepath.Join(dir, "out.docx")
	if _, err := Edit(src, out, map[string]any{"operation": "image-replace", "image": float64(0), "path": replacementPath}, dir); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := p.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	blips := office.All(doc.Root(), nsA, "blip")
	if len(blips) != 1 {
		t.Fatalf("got %d blips, want 1", len(blips))
	}
	if got := blips[0].SelectAttrValue("r:link", ""); got != "" {
		t.Fatalf("external link remains: %q", got)
	}
	embedID := blips[0].SelectAttrValue("r:embed", "")
	if embedID == "" || embedID == oldEmbedID {
		t.Fatalf("replacement embed not set: %q", embedID)
	}
	rels, err := p.Rels("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	var mediaPart string
	for _, rel := range rels {
		if rel.ID == oldLinkID || rel.Target == "https://example.test/original.png" {
			t.Fatalf("obsolete external image relationship remains: %+v", rel)
		}
		if rel.ID == embedID {
			mediaPart = opc.Resolve("word/document.xml", rel.Target)
		}
	}
	if mediaPart == "" {
		t.Fatalf("replacement relationship %q missing", embedID)
	}
	got := p.Get(mediaPart)
	if !p.Has(mediaPart) || !bytes.Equal(got, replacementData) {
		t.Fatal("replacement media missing or does not match input")
	}
}

func TestInsertReplaceResizeImagePreservesText(t *testing.T) {
	dir := t.TempDir()
	pic := filepath.Join(dir, "pic.png")
	f, err := os.Create(pic)
	if err != nil {
		t.Fatal(err)
	}
	im := image.NewRGBA(image.Rect(0, 0, 10, 10))
	im.Set(1, 1, color.RGBA{255, 0, 0, 255})
	png.Encode(f, im)
	f.Close()
	src := mk(t, "image", `<w:p>`+run("Keep this")+`</w:p>`, nil)
	out := filepath.Join(dir, "out.docx")
	_, err = Edit(src, out, map[string]any{"operation": "batch", "edits": []any{map[string]any{"operation": "image-insert", "paragraph": float64(0), "path": "pic.png", "width_in": float64(1)}, map[string]any{"operation": "image-resize", "image": float64(0), "width_in": float64(2), "height_in": float64(2)}}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := paras(t, out)[0]; got != "Keep this" {
		t.Fatal(got)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	exts := office.All(d.Root(), nsWP, "extent")
	if len(exts) != 1 || exts[0].SelectAttrValue("cx", "") != "1828800" {
		t.Fatal(exts)
	}
	for _, part := range p.Names() {
		if filepath.Ext(part) != ".rels" {
			continue
		}
		source := opc.SourceOf(part)
		rs, err := p.Rels(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rs {
			if r.TargetMode != "External" && !p.Has(opc.Resolve(source, r.Target)) {
				t.Fatal("dangling image relation", r)
			}
		}
	}
}
func TestTableColumnAndMergeEdits(t *testing.T) {
	src := mk(t, "table", `<w:tbl><w:tblPr/><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="1000"/></w:tblGrid><w:tr><w:tc><w:p>`+run("A")+`</w:p></w:tc><w:tc><w:p>`+run("B")+`</w:p></w:tc></w:tr></w:tbl>`, nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	_, err := Edit(src, out, map[string]any{"operation": "batch", "edits": []any{map[string]any{"operation": "table-column-insert", "table": float64(0), "column": float64(1)}, map[string]any{"operation": "table-column-delete", "table": float64(0), "column": float64(1)}, map[string]any{"operation": "table-merge", "table": float64(0), "row": float64(0), "column": float64(0), "end_column": float64(1)}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := opc.Open(out)
	d, _ := p.XML("word/document.xml")
	if len(d.FindElements("//w:tc")) != 1 || d.FindElement("//w:gridSpan").SelectAttrValue("w:val", "") != "2" {
		t.Fatal("merge failed")
	}
	if office.Text(d.Root(), nsW, "t") != "AB" {
		t.Fatal("text lost")
	}
}

func TestImageEditsSkipNonPictureDrawings(t *testing.T) {
	dir := t.TempDir()
	picture := filepath.Join(dir, "picture.png")
	f, err := os.Create(picture)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 10, 10))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	shape := `<w:r><w:drawing><wp:inline xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wp:extent cx="914400" cy="914400"/><wp:docPr id="42" name="Keep shape"/><wp:cNvGraphicFramePr/><a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wps:wsp><wps:cNvSpPr/><wps:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="914400" cy="914400"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></wps:spPr><wps:bodyPr/></wps:wsp></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
	src := mk(t, "mixed-drawings", `<w:p>`+shape+run("Keep text")+`</w:p>`, nil)
	mixed := filepath.Join(dir, "mixed.docx")
	if _, err := Edit(src, mixed, map[string]any{"operation": "image-insert", "paragraph": float64(0), "path": picture}, ""); err != nil {
		t.Fatal(err)
	}
	before, err := opc.Open(mixed)
	if err != nil {
		t.Fatal(err)
	}
	bdoc, err := before.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	oldID := office.All(bdoc.Root(), nsA, "blip")[0].SelectAttrValue("r:embed", "")
	for _, op := range []string{"image-delete", "image-resize", "image-replace"} {
		t.Run(op, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.docx")
			spec := map[string]any{"operation": op, "image": float64(0), "path": picture, "width_in": float64(2), "height_in": float64(2)}
			if _, err := Edit(mixed, out, spec, ""); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML("word/document.xml")
			if err != nil {
				t.Fatal(err)
			}
			shapes := office.All(d.Root(), "http://schemas.microsoft.com/office/word/2010/wordprocessingShape", "wsp")
			if len(shapes) != 1 {
				t.Fatal("non-picture shape removed")
			}
			if office.All(shapes[0], nsA, "ext")[0].SelectAttrValue("cx", "") != "914400" {
				t.Fatal("non-picture shape resized")
			}
			extents := office.All(d.Root(), nsWP, "extent")
			if extents[0].SelectAttrValue("cx", "") != "914400" {
				t.Fatal("non-picture drawing extent changed")
			}
			pics := office.All(d.Root(), nsPic, "pic")
			if op == "image-delete" {
				if len(pics) != 0 {
					t.Fatal("picture not deleted")
				}
			} else {
				if len(pics) != 1 {
					t.Fatal("picture missing")
				}
				if extents[1].SelectAttrValue("cx", "") != "1828800" {
					t.Fatal("picture not resized")
				}
				if op == "image-replace" {
					id := office.All(pics[0], nsA, "blip")[0].SelectAttrValue("r:embed", "")
					if id == oldID {
						t.Fatal("picture relationship not replaced")
					}
					rels, err := p.Rels("word/document.xml")
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, r := range rels {
						if r.ID == id && p.Has(opc.Resolve("word/document.xml", r.Target)) {
							found = true
						}
					}
					if !found {
						t.Fatal("replacement image missing")
					}
				}
			}
			if paras(t, out)[0] != "Keep text" {
				t.Fatal("text changed")
			}
			// A drawing-only document must not silently accept an image operation.
			rejected := filepath.Join(t.TempDir(), "rejected.docx")
			if _, err := Edit(src, rejected, spec, ""); err == nil {
				t.Fatal("accepted shape as picture")
			}
			if _, err := os.Stat(rejected); !os.IsNotExist(err) {
				t.Fatal("rejected operation published file", err)
			}
		})
	}
}
