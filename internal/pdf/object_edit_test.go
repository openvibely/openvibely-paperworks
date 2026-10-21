package pdf

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceArtworkPreservesText(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	art := filepath.Join(dir, "art.pdf")
	out := filepath.Join(dir, "out.pdf")
	create := func(file string, els []CanvasElement) {
		t.Helper()
		if err := Create(ctx, CreateSpec{Canvas: &Canvas{Pages: []CanvasPage{{Width: 200, Height: 200, Elements: els}}}}, dir, file); err != nil {
			t.Fatal(err)
		}
	}
	create(in, []CanvasElement{{Type: "text", Text: "Preserve this title", X: 10, Y: 20}, {Type: "rect", X: 50, Y: 50, W: 50, H: 50, Fill: "FF0000"}})
	create(art, []CanvasElement{{Type: "ellipse", X: 50, Y: 50, W: 50, H: 50, Fill: "0000FF"}})
	before, _ := os.ReadFile(in)
	objs, err := Objects(ctx, before)
	if err != nil {
		t.Fatal(err)
	}
	remove := []int{}
	for _, o := range objs {
		if o.Type == 2 {
			remove = append(remove, o.Index)
		}
	}
	if len(remove) == 0 {
		t.Fatal("no path objects")
	}
	if err = EditObjects(ctx, in, out, dir, ObjectEditSpec{Edits: []ObjectEdit{{Page: 1, Remove: remove, OverlayPDF: art}}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	text, err := PageText(ctx, data)
	if err != nil || len(text) != 1 || !strings.Contains(text[0], "Preserve this title") {
		t.Fatal(text, err)
	}
	after, _ := os.ReadFile(in)
	if string(before) != string(after) {
		t.Fatal("source changed")
	}
	objects, err := Objects(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	hasForm := false
	for _, o := range objects {
		if o.Type == 5 {
			hasForm = true
		}
	}

	_, err = RenderImages(ctx, data, RenderOptions{DPI: 72}, func(page int, img *image.RGBA) error {
		r, g, b, _ := img.At(75, 75).RGBA()
		if b < 60000 || r > 1000 || g > 1000 {
			t.Errorf("replacement artwork not blue: %d %d %d", r, g, b)
		}
		r, g, b, _ = img.At(51, 51).RGBA()
		if r < 60000 || g < 60000 || b < 60000 {
			t.Errorf("old red rectangle remains: %d %d %d", r, g, b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasForm {
		t.Fatal("replacement not embedded as native form", objects)
	}
}
