package wordedit

import (
	"path"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// UNRUN. Mirrors reference DOCX library 1.2.0's package.parts graph walk and its
// registered XmlPart subclasses. Generic XML and ImagePart have no element.
// Traversal continues through those parts; ZIP membership alone is insufficient.
func referenceElementParts(t *testing.T, pkg *opc.Package) []*etree.Element {
	t.Helper()
	types, err := pkg.XML("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	defaults, overrides := map[string]string{}, map[string]string{}
	for _, node := range types.Root().ChildElements() {
		switch node.Tag {
		case "Default":
			defaults[strings.ToLower(node.SelectAttrValue("Extension", ""))] = node.SelectAttrValue("ContentType", "")
		case "Override":
			overrides[strings.ToLower(node.SelectAttrValue("PartName", ""))] = node.SelectAttrValue("ContentType", "")
		}
	}
	xmlTypes := map[string]bool{
		"application/vnd.openxmlformats-package.core-properties+xml": true,
		ctBase + "comments+xml":      true,
		ctBase + "document.main+xml": true,
		ctBase + "footer+xml":        true,
		ctBase + "header+xml":        true,
		ctBase + "numbering+xml":     true,
		ctBase + "settings+xml":      true,
		ctBase + "styles+xml":        true,
	}
	var result []*etree.Element
	seen := map[string]bool{}
	var visit func(string)
	visit = func(source string) {
		rels, err := pkg.Rels(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, rel := range rels {
			if rel.TargetMode == "External" {
				continue
			}
			name := referencePartTarget(source, rel.Target)
			if seen[name] {
				continue
			}
			seen[name] = true
			if !pkg.Has(name) {
				t.Fatalf("missing relationship target %s", name)
			}
			contentType := overrides[strings.ToLower("/"+name)]
			if contentType == "" {
				contentType = defaults[strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))]
			}
			if contentType == "" {
				t.Fatalf("missing content type for %s", name)
			}
			if xmlTypes[contentType] && rel.Type != relBase+"image" {
				doc, err := pkg.XML(name)
				if err != nil {
					t.Fatal(err)
				}
				result = append(result, doc.Root())
			}
			visit(name)
		}
	}
	visit("")
	return result
}

// UNRUN source contract: ordered DFS, cycles/shared targets, reachable generic
// parts, content-type selection, image override, and orphan/external exclusion.
func TestReferenceElementPartsDiscoveryContract(t *testing.T) {
	source := mk(t, "package-discovery", "<w:p/>", nil)
	pkg, err := opc.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	pkg.Set("_rels/.rels", []byte(`<Relationships xmlns="`+opc.NSRel+`"><Relationship Id="r1" Type="`+relBase+`officeDocument" Target="word/document.xml"/></Relationships>`))
	pkg.Set("word/_rels/document.xml.rels", []byte(`<Relationships xmlns="`+opc.NSRel+`"/>`))
	for _, spec := range []struct{ name, typ, relation string }{
		{"renamed.xml", ctBase + "header+xml", "header"},
		{"generic.xml", "application/xml", "customXml"},
		{"notes.xml", ctBase + "footnotes+xml", "footnotes"},
		{"image.xml", ctBase + "header+xml", "image"},
		{"orphan.xml", ctBase + "header+xml", ""},
	} {
		pkg.Set("word/"+spec.name, []byte(`<w:hdr `+wNS+`><w:sdt><w:sdtPr><w:tag w:val="`+spec.name+`"/></w:sdtPr></w:sdt></w:hdr>`))
		if err := pkg.EnsureContentType("word/"+spec.name, spec.typ); err != nil {
			t.Fatal(err)
		}
		if spec.relation != "" {
			if _, err := pkg.AddRel("word/document.xml", relBase+spec.relation, spec.name, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A second typed story reachable only through a generic part is included.
	pkg.Set("word/nested.xml", []byte(`<w:ftr `+wNS+`><w:sdt><w:sdtPr><w:tag w:val="nested.xml"/></w:sdtPr></w:sdt></w:ftr>`))
	if err := pkg.EnsureContentType("word/nested.xml", ctBase+"footer+xml"); err != nil {
		t.Fatal(err)
	}
	for _, edge := range []struct{ source, target string }{
		{"word/generic.xml", "nested.xml"},
		{"word/nested.xml", "document.xml"},
		{"word/notes.xml", "renamed.xml"},
	} {
		if _, err := pkg.AddRel(edge.source, relBase+"header", edge.target, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pkg.AddRel("word/document.xml", relBase+"header", "https://example.invalid/external.xml", true); err != nil {
		t.Fatal(err)
	}
	var tags []string
	roots := referenceElementParts(t, pkg)
	referenceEqual(t, len(roots), 3) // document, renamed header, nested footer
	for _, root := range roots {
		for _, tag := range office.All(root, nsW, "tag") {
			tags = append(tags, tag.SelectAttrValue("w:val", ""))
		}
	}
	referenceEqual(t, tags, []string{"renamed.xml", "nested.xml"})
}

// UNRUN. PackURI.from_rel_ref uses POSIX join/abspath, not URI unescaping
// or fragment removal. The result here is the ZIP membername (one slash removed).
func referencePartTarget(source, target string) string {
	joined := target
	if !strings.HasPrefix(target, "/") {
		base := "/" + source
		base = base[:strings.LastIndex(base, "/")+1]
		joined = base + target
	}
	cleaned := path.Clean(joined)
	// posixpath.normpath preserves exactly two leading slashes.
	if strings.HasPrefix(joined, "//") && !strings.HasPrefix(joined, "///") {
		cleaned = "/" + cleaned
	}
	return strings.TrimPrefix(cleaned, "/")
}

func TestReferencePartTargetContract(t *testing.T) {
	// UNRUN: source-derived PackURI.from_rel_ref/membername expectations.
	for _, tc := range []struct{ name, source, target, want string }{
		{"package-relative", "", "word/document.xml", "word/document.xml"},
		{"sibling", "word/document.xml", "header1.xml", "word/header1.xml"},
		{"parent", "word/document.xml", "../docProps/core.xml", "docProps/core.xml"},
		{"absolute", "word/document.xml", "/docProps/core.xml", "docProps/core.xml"},
		{"percent-literal", "word/document.xml", "header%20one.xml", "word/header%20one.xml"},
		{"fragment-literal", "word/document.xml", "header.xml#one", "word/header.xml#one"},
		{"dot-segments", "word/document.xml", "./nested/../header.xml", "word/header.xml"},
		{"double-slash", "word/document.xml", "//word/header.xml", "/word/header.xml"},
		{"triple-slash", "word/document.xml", "///word/header.xml", "word/header.xml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			referenceEqual(t, referencePartTarget(tc.source, tc.target), tc.want)
		})
	}
}

func TestReferenceElementPartsFirstEncounterContract(t *testing.T) {
	// UNRUN. The reader chooses a part class from its first incoming relationship;
	// a later relationship to that same part cannot reclassify it.
	for _, tc := range []struct {
		name, first, second string
		want                int
	}{
		{"image-first", "image", "header", 1},
		{"xml-first", "header", "image", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := opc.Open(mk(t, "first-encounter", "<w:p/>", nil))
			if err != nil {
				t.Fatal(err)
			}
			pkg.Set("_rels/.rels", []byte(`<Relationships xmlns="`+opc.NSRel+`"><Relationship Id="r1" Type="`+relBase+`officeDocument" Target="word/document.xml"/></Relationships>`))
			pkg.Set("word/_rels/document.xml.rels", []byte(`<Relationships xmlns="`+opc.NSRel+`"><Relationship Id="r1" Type="`+relBase+tc.first+`" Target="header%20one.xml"/><Relationship Id="r2" Type="`+relBase+tc.second+`" Target="header%20one.xml"/></Relationships>`))
			pkg.Set("word/header%20one.xml", []byte(`<w:hdr `+wNS+`/>`))
			if err := pkg.EnsureContentType("word/header%20one.xml", ctBase+"header+xml"); err != nil {
				t.Fatal(err)
			}
			roots := referenceElementParts(t, pkg)
			referenceEqual(t, len(roots), tc.want)
			referenceEqual(t, roots[0].Tag, "document")
			if tc.want == 2 {
				referenceEqual(t, roots[1].Tag, "hdr")
			}
		})
	}
}
