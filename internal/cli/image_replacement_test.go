package cli

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestReplaceSVGBackedPicturesThroughCLI(t *testing.T) {
	const svgNS = "http://schemas.microsoft.com/office/drawing/2016/SVG/main"
	for _, kind := range []string{"docx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			for name, c := range map[string]color.RGBA{"red": {255, 0, 0, 255}, "green": {0, 255, 0, 255}} {
				im := image.NewRGBA(image.Rect(0, 0, 64, 64))
				for y := 0; y < 64; y++ {
					for x := 0; x < 64; x++ {
						im.SetRGBA(x, y, c)
					}
				}
				var b bytes.Buffer
				if err := png.Encode(&b, im); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name+".png"), b.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
			seed := filepath.Join(dir, "seed."+kind)
			part, media, target := "word/document.xml", "word/media/original.svg", "media/original.svg"
			if kind == "docx" {
				base := filepath.Join(dir, "base.docx")
				s := writeSpec(t, dir, "create.json", map[string]any{"blocks": []any{map[string]any{"type": "paragraph", "text": "Keep text"}}})
				runCLI(t, "create-docx", "--spec", s, "--output", base)
				s = writeSpec(t, dir, "insert.json", map[string]any{"operation": "batch", "edits": []any{
					map[string]any{"operation": "image-insert", "paragraph": 0, "path": "red.png", "width_in": 1},
					map[string]any{"operation": "image-insert", "paragraph": 0, "path": "red.png", "width_in": 1},
				}})
				runCLI(t, "word-edit", base, "--spec", s, "--output", seed)
			} else {
				part, media, target = "ppt/slides/slide1.xml", "ppt/media/original.svg", "../media/original.svg"
				s := writeSpec(t, dir, "create.json", map[string]any{"width_in": 4, "height_in": 3, "slides": []any{map[string]any{"elements": []any{
					map[string]any{"type": "image", "path": "red.png", "x": 0.5, "y": 1, "w": 1, "h": 1},
					map[string]any{"type": "image", "path": "red.png", "x": 2, "y": 1, "w": 1, "h": 1},
				}}}})
				runCLI(t, "create-pptx", "--spec", s, "--output", seed)
			}
			p, err := opc.Open(seed)
			if err != nil {
				t.Fatal(err)
			}
			svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="blue"/></svg>`)
			p.Set(media, svg)
			if err := p.EnsureContentType(media, "image/svg+xml"); err != nil {
				t.Fatal(err)
			}
			rid, err := p.AddRel(part, office.NSR+"/image", target, false)
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.XML(part)
			if err != nil {
				t.Fatal(err)
			}
			blips := office.All(d.Root(), office.NSA, "blip")
			if len(blips) != 2 {
				t.Fatal("expected two pictures")
			}
			for _, blip := range blips {
				list := blip.CreateElement("a:extLst")
				ext := list.CreateElement("a:ext")
				ext.CreateAttr("uri", "{96DAC541-7B7A-43D3-8B79-37D633B846F1}")
				source := ext.CreateElement("vector:svgBlip")
				source.CreateAttr("xmlns:vector", svgNS)
				source.CreateAttr("xmlns:r", office.NSR)
				source.CreateAttr("r:embed", rid)
				if kind == "docx" {
					// An unrelated extension must survive replacement.
					ext = list.CreateElement("a:ext")
					ext.CreateAttr("uri", "{28A0092B-C50C-407E-A947-70E740481C1C}")
					dpi := ext.CreateElement("a14:useLocalDpi")
					dpi.CreateAttr("xmlns:a14", "http://schemas.microsoft.com/office/drawing/2010/main")
					dpi.CreateAttr("val", "0")
				}
			}
			untouched := blips[1].Copy()
			if err := p.SetXML(part, d); err != nil {
				t.Fatal(err)
			}
			in, out := filepath.Join(dir, "svg."+kind), filepath.Join(dir, "out."+kind)
			if err := p.SaveNew(in); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "docx" {
				s := writeSpec(t, dir, "replace.json", map[string]any{"operation": "image-replace", "image": 0, "path": "green.png"})
				runCLI(t, "word-edit", in, "--spec", s, "--output", out)
			} else {
				s := writeSpec(t, dir, "replace.json", map[string]any{"edits": []any{map[string]any{"operation": "replace-image", "slide": 1, "shape_id": "2", "path": "green.png"}}})
				runCLI(t, "slide-edit", in, "--spec", s, "--output", out)
			}
			p, err = opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			d, err = p.XML(part)
			if err != nil {
				t.Fatal(err)
			}
			blips = office.All(d.Root(), office.NSA, "blip")
			if len(office.All(blips[0], svgNS, "svgBlip")) != 0 {
				t.Fatal("replacement still selects old SVG artwork")
			}
			if kind == "docx" && len(office.All(blips[0], "http://schemas.microsoft.com/office/drawing/2010/main", "useLocalDpi")) != 1 {
				t.Fatal("unrelated extension removed")
			}
			if kind == "pptx" && office.Child(blips[0], office.NSA, "extLst") != nil {
				t.Fatal("empty SVG extension wrapper retained")
			}
			beforeDoc, afterDoc := etree.NewDocument(), etree.NewDocument()
			beforeDoc.SetRoot(untouched)
			afterDoc.SetRoot(blips[1].Copy())
			before, _ := beforeDoc.WriteToString()
			after, _ := afterDoc.WriteToString()
			if before != after || !bytes.Equal(p.Get(media), svg) {
				t.Fatal("untouched picture or shared SVG changed")
			}
			current, err := os.ReadFile(in)
			if err != nil || !bytes.Equal(current, original) {
				t.Fatal("input changed", err)
			}
			preview := filepath.Join(dir, "preview")
			runCLI(t, "render", out, "--output", preview)
			f, err := os.Open(filepath.Join(preview, "page-0001.png"))
			if err != nil {
				t.Fatal(err)
			}
			im, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			green, blue, red := 0, 0, 0
			for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
				for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
					r, g, b, _ := im.At(x, y).RGBA()
					if g > 50000 && r < 10000 && b < 10000 {
						green++
					}
					if b > 50000 && r < 10000 && g < 10000 {
						blue++
					}
					if r > 50000 && g < 10000 && b < 10000 {
						red++
					}
				}
			}
			if green < 1000 || blue < 1000 || red != 0 {
				t.Fatalf("expected green replacement and blue untouched SVG: green=%d blue=%d red=%d", green, blue, red)
			}
		})
	}
}
