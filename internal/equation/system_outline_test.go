package equation

import (
	"github.com/luo-studio/go-tex/tex/fonts"
	"github.com/luo-studio/go-tex/tex/path"
	"testing"
)

func TestSystemOutlinePrimitive(t *testing.T) {
	data, err := fonts.TTF("Main-Regular")
	if err != nil {
		t.Fatal(err)
	}
	commands, advance, err := systemOutline(data, 0, 'A')
	if err != nil {
		t.Fatal(err)
	}
	if advance <= 0 || len(commands) < 4 {
		t.Fatalf("missing outline/advance: %v %v", advance, commands)
	}
	open := false
	for _, c := range commands {
		if c.Kind == path.KindMoveTo {
			if open {
				t.Fatal("unclosed contour")
			}
			open = true
		}
		if c.Kind == path.KindClose {
			if !open {
				t.Fatal("close without contour")
			}
			open = false
		}
	}
	if open {
		t.Fatal("final contour not closed")
	}
	for _, c := range []struct {
		name   string
		data   []byte
		face   int
		scalar rune
	}{
		{"missing scalar", data, 0, 0x1f600}, {"missing face", data, 1, 'A'},
		{"negative face", data, -1, 'A'}, {"surrogate", data, 0, 0xd800},
		{"out of range", data, 0, 0x110000}, {"empty", nil, 0, 'A'},
		{"corrupt", []byte("not an OpenType font"), 0, 'A'},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := systemOutline(c.data, c.face, c.scalar); err == nil {
				t.Fatal("invalid or absent glyph accepted")
			}
		})
	}
}
