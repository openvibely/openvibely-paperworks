package opc

import "testing"

func TestRelative(t *testing.T) {
	for _, tt := range []struct {
		source, target, want string
	}{
		{"ppt/slides/slide1.xml", "ppt/media/image1.png", "../media/image1.png"},
		{"xl/worksheets/sheet1.xml", "xl/drawings/drawing1.xml", "../drawings/drawing1.xml"},
		{"xl/workbook.xml", "xl/persons/person.xml", "persons/person.xml"},
		{"word/document.xml", "word/media/a.png", "media/a.png"},
		{"ppt/slides/slide1.xml", "ppt/slides/slide2.xml", "slide2.xml"},
		{"word/document.xml", "word/document.xml", "document.xml"},
		{"ppt/slides/slide1.xml", "ppt/slides", "../slides"},
		{"word/document.xml", "custom/item.xml", "../custom/item.xml"},
	} {
		t.Run(tt.source+"->"+tt.target, func(t *testing.T) {
			got := Relative(tt.source, tt.target)
			if got != tt.want {
				t.Fatalf("Relative = %q, want %q", got, tt.want)
			}
			if resolved := Resolve(tt.source, got); resolved != tt.target {
				t.Errorf("Resolve = %q, want %q", resolved, tt.target)
			}
		})
	}
}

func TestRelativePreservesExistingPathBehavior(t *testing.T) {
	for _, tt := range []struct {
		source, target, want string
	}{
		// Preserve the existing helpers' treatment of path.Dir(source) == ".".
		{"document.xml", "media/a.png", "../media/a.png"},
		{"", "word/document.xml", "../word/document.xml"},
		{"word/document.xml", "word/media/a b%20.png", "media/a b%20.png"},
		{"word/document.xml", "word/media/../a.png", "media/../a.png"},
	} {
		t.Run(tt.source+"->"+tt.target, func(t *testing.T) {
			if got := Relative(tt.source, tt.target); got != tt.want {
				t.Errorf("Relative = %q, want %q", got, tt.want)
			}
		})
	}
}
