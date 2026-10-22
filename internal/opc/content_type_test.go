package opc

import (
	"fmt"
	"strings"
	"testing"
)

func ctPackage(types string) *Package {
	p := &Package{index: map[string]*Part{}}
	p.Set(contentTypesName, []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+`<Types xmlns="`+NSCT+`">`+types+`</Types>`))
	return p
}

func wantCT(t *testing.T, p *Package, name, want string) {
	t.Helper()
	if got := p.ContentType(name); got != want {
		t.Fatalf("ContentType(%q) = %q, want %q", name, got, want)
	}
}

func TestContentTypeEnsureVisibleImmediately(t *testing.T) {
	p := ctPackage(`<Default Extension="xml" ContentType="application/xml"/>`)
	wantCT(t, p, "media/a.png", "")
	if err := p.EnsureContentType(".png", "image/png"); err != nil {
		t.Fatal(err)
	}
	wantCT(t, p, "media/a.png", "image/png")
	wantCT(t, p, "word/x.xml", "application/xml")
	if err := p.EnsureContentType("word/x.xml", "application/x+xml"); err != nil {
		t.Fatal(err)
	}
	wantCT(t, p, "word/x.xml", "application/x+xml")
	if err := p.EnsureContentType("/word/x.xml", "application/y+xml"); err != nil {
		t.Fatal(err)
	}
	wantCT(t, p, "word/x.xml", "application/y+xml")
}

func TestContentTypeSetInvalidates(t *testing.T) {
	p := ctPackage(`<Default Extension="xml" ContentType="application/xml"/>`)
	wantCT(t, p, "a.xml", "application/xml")
	p.Set(contentTypesName, []byte(`<Types xmlns="`+NSCT+`"><Default Extension="xml" ContentType="text/xml"/></Types>`))
	wantCT(t, p, "a.xml", "text/xml")
	doc, err := p.XML(contentTypesName)
	if err != nil {
		t.Fatal(err)
	}
	doc.Root().SelectElement("Default").CreateAttr("ContentType", "application/z+xml")
	if err := p.SetXML(contentTypesName, doc); err != nil {
		t.Fatal(err)
	}
	wantCT(t, p, "a.xml", "application/z+xml")
	p.index[contentTypesName].Data = []byte(`<Types xmlns="` + NSCT + `"><Default Extension="xml" ContentType="direct/xml"/></Types>`)
	wantCT(t, p, "a.xml", "direct/xml")
}

func TestContentTypeOverrideBeatsDefault(t *testing.T) {
	p := ctPackage(`<Override PartName="/word/document.xml" ContentType="main"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/Other.xml" ContentType="other"/>`)
	wantCT(t, p, "word/document.xml", "main")
	wantCT(t, p, "word/styles.xml", "application/xml")
	wantCT(t, p, "word/other.xml", "application/xml")
	wantCT(t, p, "word/Other.xml", "other")
}

func TestContentTypeFirstMatchWins(t *testing.T) {
	p := ctPackage(`<Default Extension="png" ContentType="first"/><Default Extension="PNG" ContentType="second"/><Override PartName="/a.xml" ContentType="o1"/><Override PartName="a.xml" ContentType="o2"/>`)
	wantCT(t, p, "x.png", "first")
	wantCT(t, p, "a.xml", "o1")
}

func TestContentTypeExtensionCaseInsensitive(t *testing.T) {
	p := ctPackage(`<Default Extension="PNG" ContentType="image/png"/><Default Extension="jpeg" ContentType="image/jpeg"/>`)
	wantCT(t, p, "media/a.png", "image/png")
	wantCT(t, p, "media/b.JPEG", "image/jpeg")
	wantCT(t, p, "media/c", "")
}

func TestContentTypeMissingOrMalformed(t *testing.T) {
	p := &Package{index: map[string]*Part{}}
	wantCT(t, p, "a.xml", "")
	p.Set(contentTypesName, []byte(`<Types`))
	wantCT(t, p, "a.xml", "")
	p.Set(contentTypesName, []byte(`<!DOCTYPE x><Types/>`))
	wantCT(t, p, "a.xml", "")
	p.Set(contentTypesName, []byte(`<Types xmlns="`+NSCT+`"><Default Extension="xml" ContentType="application/xml"/></Types>`))
	wantCT(t, p, "a.xml", "application/xml")
	p.Remove(contentTypesName)
	wantCT(t, p, "a.xml", "")
	p.Set(contentTypesName, []byte(`<Types xmlns="`+NSCT+`"><Default Extension="xml" ContentType="again"/></Types>`))
	wantCT(t, p, "a.xml", "again")
}

func TestContentTypePartRemovedAndReadded(t *testing.T) {
	p := ctPackage(`<Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="main"/>`)
	p.Set("word/document.xml", []byte("<a/>"))
	wantCT(t, p, "word/document.xml", "main")
	p.Remove("word/document.xml")
	wantCT(t, p, "word/document.xml", "main")
	p.Set("word/document.xml", []byte("<b/>"))
	wantCT(t, p, "word/document.xml", "main")
}

func benchPackage(n int) *Package {
	var types strings.Builder
	types.WriteString(`<Default Extension="xml" ContentType="application/xml"/><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="png" ContentType="image/png"/>`)
	p := &Package{index: map[string]*Part{}}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("ppt/slides/slide%d.xml", i)
		if i%2 == 1 {
			name = fmt.Sprintf("ppt/media/image%d.png", i)
		} else {
			fmt.Fprintf(&types, `<Override PartName="/%s" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, name)
		}
		p.Set(name, []byte("<x/>"))
	}
	p.Set(contentTypesName, []byte(`<Types xmlns="`+NSCT+`">`+types.String()+`</Types>`))
	return p
}

func BenchmarkContentTypeAllParts(b *testing.B) {
	for _, n := range []int{100, 500, 2000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			src := benchPackage(n)
			raw, err := src.Bytes()
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				p, err := Read(raw)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				for _, part := range p.Parts {
					_ = p.ContentType(part.Name)
				}
			}
		})
	}
}
