package testsupport

import (
	"fmt"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// OrderPresentationFixture follows pinned reference presentation library OpcPackage.iter_parts and
// PackageWriter._write_parts for the reachable runtime470 fixture. It changes
// only ZIP member order, not relationship XML, content types, or payload bytes.
// Unreachable or empty-relationship parts are rejected rather than dropped:
// their general reference presentation library load/save behavior is outside this fixture adapter.
// The production OPC writer retains its original-order preservation contract.
func OrderPresentationFixture(p *opc.Package) error {
	byName := make(map[string]*opc.Part, len(p.Parts))
	for _, part := range p.Parts {
		byName[part.Name] = part
	}
	var ordered []*opc.Part
	written := make(map[string]bool, len(p.Parts))
	appendPart := func(name string) error {
		part := byName[name]
		if part == nil {
			return fmt.Errorf("presentation fixture part missing: %s", name)
		}
		if written[name] {
			return fmt.Errorf("presentation fixture repeated physical part: %s", name)
		}
		written[name] = true
		ordered = append(ordered, part)
		return nil
	}
	if err := appendPart("[Content_Types].xml"); err != nil {
		return err
	}
	if err := appendPart("_rels/.rels"); err != nil {
		return err
	}
	visited := map[string]bool{}
	var walk func(string) error
	walk = func(source string) error {
		rels, err := p.Rels(source)
		if err != nil {
			return err
		}
		for _, rel := range rels {
			if rel.TargetMode == "External" {
				continue
			}
			target := opc.Resolve(source, rel.Target)
			if visited[target] {
				continue
			}
			visited[target] = true
			if err := appendPart(target); err != nil {
				return err
			}
			children, err := p.Rels(target)
			if err != nil {
				return err
			}
			if len(children) != 0 {
				if err := appendPart(opc.RelsName(target)); err != nil {
					return err
				}
			}
			if err := walk(target); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return err
	}
	if len(ordered) != len(p.Parts) {
		return fmt.Errorf("presentation fixture has unreachable or empty-relationship parts: ordered %d of %d", len(ordered), len(p.Parts))
	}
	p.Parts = ordered
	return nil
}
