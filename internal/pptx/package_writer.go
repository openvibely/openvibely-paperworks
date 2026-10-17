package pptx

import (
	"encoding/json"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"sort"
	"time"
)

// presentationBytes follows the pinned OPC writer's depth-first relationship
// traversal. Relationships are emitted immediately after their source part.
// Orphan parts retain the general OPC path, rather than being silently dropped.
func presentationBytes(p *opc.Package, clock func() time.Time) ([]byte, error) {
	raw, err := chartWorkbookAssets.ReadFile("assets/chart-workbook/template-rel-order.json")
	if err != nil {
		return nil, err
	}
	orders := map[string][]string{}
	if err := json.Unmarshal(raw, &orders); err != nil {
		return nil, err
	}
	out := &opc.Package{}
	seen := map[string]bool{}
	appendPart := func(name string) error {
		if seen[name] {
			return nil
		}
		if !p.Has(name) {
			return fmt.Errorf("missing presentation part %s", name)
		}
		seen[name] = true
		out.Parts = append(out.Parts, &opc.Part{Name: name, Data: p.Get(name)})
		return nil
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels"} {
		if err := appendPart(name); err != nil {
			return nil, err
		}
	}
	var walk func(string) error
	walk = func(source string) error {
		rels, err := p.Rels(source)
		if err != nil {
			return err
		}
		relname := opc.RelsName(source)
		if source == "" {
			relname = "_rels/.rels"
		}
		// Loading the template preserves insertion order; its serialized .rels
		// uses numeric order instead. New relationships follow the loaded entries.
		rank := map[string]int{}
		for i, id := range orders[relname] {
			rank[id] = i
		}
		sort.SliceStable(rels, func(i, j int) bool {
			a, oka := rank[rels[i].ID]
			b, okb := rank[rels[j].ID]
			if oka != okb {
				return oka
			}
			return oka && a < b
		})
		for _, r := range rels {
			if r.TargetMode == "External" {
				continue
			}
			name := opc.Resolve(source, r.Target)
			if seen[name] {
				continue
			}
			if err := appendPart(name); err != nil {
				return err
			}
			if rel := opc.RelsName(name); p.Has(rel) {
				if err := appendPart(rel); err != nil {
					return err
				}
			}
			if err := walk(name); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return nil, err
	}
	if len(seen) != len(p.Parts) {
		return p.Bytes()
	}
	return out.DocumentBytes(clock)
}
