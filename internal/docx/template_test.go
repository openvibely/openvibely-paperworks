package docx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// The hashes are captured from the unchanged pinned Word example and separately
// checked against a fresh reference DOCX library Document(). Never captured report content.
func TestSharedTemplateResourcesAndRelationships(t *testing.T) {
	raw, err := os.ReadFile("testdata/template-parts.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Parts map[string]string
		Names []string
	}
	if err := json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	for _, populated := range []bool{false, true} {
		name := "blank"
		if populated {
			name = "authored-resources"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			spec := Spec{Page: Page{WidthIn: 8.5, HeightIn: 11, MarginIn: 1}, Font: "Arial", FontSize: 11}
			if populated {
				f, e := os.Create(filepath.Join(dir, "pixel.png"))
				if e != nil {
					t.Fatal(e)
				}
				if e = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); e != nil {
					t.Fatal(e)
				}
				if e = f.Close(); e != nil {
					t.Fatal(e)
				}
				spec.Header = "Header"
				spec.Footer = "Footer"
				spec.PageNumbers = true
				spec.Blocks = []Block{{Type: "paragraph", Runs: []Run{{Text: "link", Link: "https://example.com/"}}}, {Type: "image", Path: "pixel.png"}, {Type: "list", Ordered: true, Items: []string{"one", "two"}}}
			}
			raw, e := Build(spec, dir)
			if e != nil {
				t.Fatal(e)
			}
			p, e := opc.Read(raw)
			if e != nil {
				t.Fatal(e)
			}
			for n, h := range oracle.Parts {
				sum := sha256.Sum256(p.Get(n))
				if hex.EncodeToString(sum[:]) != h {
					t.Errorf("template payload changed or missing: %s", n)
				}
			}
			for _, n := range oracle.Names {
				if !p.Has(n) {
					t.Errorf("reference member missing: %s", n)
				}
			}
			if !populated && len(p.Parts) != len(oracle.Names) {
				t.Fatal("unexpected template members", len(p.Parts))
			}
			// Walk every relationship owner, not just the document, including the custom
			// XML property link and root thumbnail. External links are never dereferenced.
			for _, part := range p.Parts {
				if !strings.HasSuffix(part.Name, ".rels") {
					continue
				}
				owner := opc.SourceOf(part.Name)
				rs, e := p.Rels(owner)
				if e != nil {
					t.Fatal(e)
				}
				seen := map[string]bool{}
				for _, r := range rs {
					if seen[r.ID] {
						t.Fatal("duplicate relationship ID", owner, r.ID)
					}
					seen[r.ID] = true
					if r.TargetMode != "External" && !p.Has(opc.Resolve(owner, r.Target)) {
						t.Fatal("dangling relationship", owner, r)
					}
				}
			}
			rs, e := p.Rels("word/document.xml")
			if e != nil {
				t.Fatal(e)
			}
			ids := map[string]bool{}
			types := map[string]int{}
			for _, r := range rs {
				ids[r.ID] = true
				types[r.Type]++
			}
			if types[opc.NSOffR+"/theme"] != 1 {
				t.Fatal("theme missing or duplicated")
			}
			if populated {
				for _, typ := range []string{"hyperlink", "image", "header", "footer"} {
					if types[opc.NSOffR+"/"+typ] != 1 {
						t.Fatal("authored relationship missing", typ)
					}
				}
				d, e := p.XML("word/document.xml")
				if e != nil {
					t.Fatal(e)
				}
				for _, el := range d.FindElements("//*") {
					for _, a := range el.Attr {
						if a.Space == "r" && !ids[a.Value] {
							t.Fatal("unresolved document reference", a)
						}
					}
				}
			}
			// Every resource has a declared content type; relationships alone do not
			// establish a valid OPC package.
			ct, e := p.XML("[Content_Types].xml")
			if e != nil {
				t.Fatal(e)
			}
			defaults, overrides := map[string]bool{}, map[string]bool{}
			for _, el := range ct.Root().ChildElements() {
				if el.Tag == "Default" {
					defaults[el.SelectAttrValue("Extension", "")] = true
				} else {
					overrides[el.SelectAttrValue("PartName", "")] = true
				}
			}
			for _, part := range p.Parts {
				if part.Name == "[Content_Types].xml" {
					continue
				}
				ext := strings.TrimPrefix(filepath.Ext(part.Name), ".")
				if !defaults[ext] && !overrides["/"+part.Name] {
					t.Fatal("missing content type", part.Name)
				}
			}
		})
	}
}
