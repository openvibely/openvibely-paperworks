package pptx

import (
	"encoding/json"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"strings"
)

// Author through a temporary deck without interpreting destination part names
// as temporary slide names. Work on a deep copy, including notes/table runs.
func deferSlideLinks(pkg *opc.Package, slide Slide) (Slide, map[string]string, error) {
	raw, err := json.Marshal(slide)
	if err != nil {
		return slide, nil, err
	}
	var value any
	if err = json.Unmarshal(raw, &value); err != nil {
		return slide, nil, err
	}
	targets := map[string]string{}
	var walk func(any) error
	walk = func(v any) error {
		switch x := v.(type) {
		case []any:
			for _, c := range x {
				if err := walk(c); err != nil {
					return err
				}
			}
		case map[string]any:
			if link, ok := x["link"].(map[string]any); ok && link["is_external"] != true {
				uri, _ := link["uri"].(string)
				if uri != "" {
					if !pkg.Has(uri) || pkg.ContentType(uri) != "application/vnd.openxmlformats-officedocument.presentationml.slide+xml" {
						return fmt.Errorf("internal link uri must be an existing destination slide part")
					}
					action, _ := link["action"].(string)
					if action != "" && action != "ppaction://hlinksldjump" {
						return fmt.Errorf("slide target requires hlinksldjump action")
					}
					// Unique per operation, with no dependence on user URLs.
					marker := fmt.Sprintf("https://paperworks.invalid/deferred/%s/%d", opc.Hash(raw), len(targets))
					targets[marker] = uri
					link["uri"] = marker
					link["is_external"] = true
					delete(link, "action")
				}
			}
			for _, c := range x {
				if err := walk(c); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err = walk(value); err != nil {
		return slide, nil, err
	}
	raw, err = json.Marshal(value)
	if err != nil {
		return slide, nil, err
	}
	var out Slide
	err = json.Unmarshal(raw, &out)
	return out, targets, err
}
func resolveDeferredSlideLinks(pkg *opc.Package, prior map[string]bool, targets map[string]string) error {
	if len(targets) == 0 {
		return nil
	}
	for _, name := range pkg.Names() {
		if prior[name] || !strings.HasSuffix(name, ".rels") {
			continue
		}
		part := opc.SourceOf(name)
		rels, err := pkg.XML(name)
		if err != nil {
			return err
		}
		ids := map[string]bool{}
		for _, r := range rels.Root().ChildElements() {
			target, ok := targets[r.SelectAttrValue("Target", "")]
			if !ok {
				continue
			}
			ids[r.SelectAttrValue("Id", "")] = true
			r.CreateAttr("Type", rns+"/slide")
			r.CreateAttr("Target", opc.Relative(part, target))
			r.RemoveAttr("TargetMode")
		}
		if len(ids) == 0 {
			continue
		}
		d, err := pkg.XML(part)
		if err != nil {
			return err
		}
		for _, h := range office.All(d.Root(), ans, "hlinkClick") {
			if ids[h.SelectAttrValue("r:id", "")] {
				h.CreateAttr("action", "ppaction://hlinksldjump")
			}
		}
		if err = pkg.SetXML(part, d); err != nil {
			return err
		}
		if err = pkg.SetXML(name, rels); err != nil {
			return err
		}
	}
	return nil
}
