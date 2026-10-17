package pptx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestDefaultTemplateResources(t *testing.T) {
	raw, err := os.ReadFile("testdata/template-resources.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Parts []struct{ Name, SHA256 string }
	}
	if err := json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	// Multiple slides and notes share the same presentation relationship namespace.
	s := Spec{Title: "Authored title", Slides: []Slide{{Notes: "First notes", Elements: []Element{{Type: "text", Text: "First", X: 1, Y: 1, W: 5, H: 1}}}, {Notes: "Second notes"}}}
	out := filepath.Join(t.TempDir(), "new.pptx")
	if err := Create(context.Background(), s, ".", out); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	reached := map[string]int{}
	for _, part := range p.Parts {
		if !strings.HasSuffix(part.Name, ".rels") {
			continue
		}
		source := opc.SourceOf(part.Name)
		rs, err := p.Rels(source)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, r := range rs {
			if ids[r.ID] {
				t.Fatalf("duplicate ID %s in %s", r.ID, part.Name)
			}
			ids[r.ID] = true
			if r.TargetMode == "External" {
				continue
			}
			target := opc.Resolve(source, r.Target)
			if !p.Has(target) {
				t.Fatalf("dangling %s -> %s", source, target)
			}
			reached[target]++
		}
	}
	ct, err := p.XML("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range oracle.Parts {
		if got := fmt.Sprintf("%x", sha256.Sum256(p.Get(part.Name))); got != part.SHA256 {
			t.Errorf("%s: %s != %s", part.Name, got, part.SHA256)
		}
		if reached[part.Name] != 1 {
			t.Errorf("%s: reached %d times", part.Name, reached[part.Name])
		}
		found := false
		for _, e := range ct.Root().ChildElements() {
			if e.SelectAttrValue("PartName", "") == "/"+part.Name ||
				e.SelectAttrValue("Extension", "") == strings.TrimPrefix(filepath.Ext(part.Name), ".") {
				found = true
			}
		}
		if !found {
			t.Errorf("missing content type: %s", part.Name)
		}
	}
	if !strings.Contains(string(p.Get("docProps/core.xml")), "Authored title") {
		t.Fatal("authored title lost")
	}
	// Packages must not share mutable template bytes.
	p.Set("docProps/app.xml", []byte("changed"))
	second := filepath.Join(t.TempDir(), "second.pptx")
	if err := Create(context.Background(), s, ".", second); err != nil {
		t.Fatal(err)
	}
	q, err := opc.Open(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(q.Get("docProps/app.xml")) == "changed" {
		t.Fatal("template mutated")
	}
}
