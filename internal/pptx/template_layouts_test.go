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

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestDefaultLayoutGraph(t *testing.T) {
	raw, err := os.ReadFile("testdata/template-layouts.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Parts []struct{ Name, SHA256 string }
	}
	if err = json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{0, 5, 6, 10} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			s := Spec{StyleResources: &StyleResources{TemplateLayout: &index}, Slides: []Slide{{Notes: "first"}, {Notes: "second"}}}
			out := filepath.Join(t.TempDir(), "deck.pptx")
			if err := Create(context.Background(), s, ".", out); err != nil {
				t.Fatal(err)
			}
			p, err := opc.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range oracle.Parts {
				if got := fmt.Sprintf("%x", sha256.Sum256(p.Get(part.Name))); got != part.SHA256 {
					t.Errorf("%s differs: %s", part.Name, got)
				}
			}
			for _, part := range p.Parts {
				if !strings.HasSuffix(part.Name, ".rels") {
					continue
				}
				source := opc.SourceOf(part.Name)
				rels, err := p.Rels(source)
				if err != nil {
					t.Fatal(err)
				}
				ids := map[string]bool{}
				for _, r := range rels {
					if ids[r.ID] {
						t.Fatal("duplicate relationship", source, r.ID)
					}
					ids[r.ID] = true
					if r.TargetMode != "External" && !p.Has(opc.Resolve(source, r.Target)) {
						t.Fatal("dangling relationship", source, r.Target)
					}
				}
			}
			master, err := p.XML("ppt/slideMasters/slideMaster1.xml")
			if err != nil {
				t.Fatal(err)
			}
			ids := office.All(master.Root(), pns, "sldLayoutId")
			if len(ids) != 11 {
				t.Fatalf("layout list: %d", len(ids))
			}
			rels, err := p.Rels("ppt/slideMasters/slideMaster1.xml")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				found := false
				for _, r := range rels {
					if r.ID == id.SelectAttrValue("r:id", "") && strings.HasSuffix(r.Type, "/slideLayout") {
						found = true
					}
				}
				if !found {
					t.Fatal("unresolved layout ID")
				}
			}
			for n := 1; n <= 2; n++ {
				rs, err := p.Rels(fmt.Sprintf("ppt/slides/slide%d.xml", n))
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, r := range rs {
					if strings.HasSuffix(r.Type, "/slideLayout") {
						found = true
						if r.Target != fmt.Sprintf("../slideLayouts/slideLayout%d.xml", index+1) {
							t.Fatal("wrong selected layout", r.Target)
						}
					}
				}
				if !found {
					t.Fatal("missing selected layout")
				}
			}
		})
	}
	for _, index := range []int{-1, 11} {
		t.Run(fmt.Sprintf("invalid%d", index), func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "bad.pptx")
			if Create(context.Background(), Spec{StyleResources: &StyleResources{TemplateLayout: &index}, Slides: []Slide{{}}}, ".", out) == nil {
				t.Fatal("accepted invalid layout")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("created output")
			}
		})
	}
	for _, field := range []string{"master", "layout"} {
		t.Run("conflict-"+field, func(t *testing.T) {
			index := 5
			styles := &StyleResources{TemplateLayout: &index}
			if field == "master" {
				styles.Master = "custom.xml"
			} else {
				styles.Layout = "custom.xml"
			}
			if err := Create(context.Background(), Spec{StyleResources: styles, Slides: []Slide{{}}}, ".", filepath.Join(t.TempDir(), "bad.pptx")); err == nil {
				t.Fatal("accepted ambiguous template")
			}
		})
	}
}
