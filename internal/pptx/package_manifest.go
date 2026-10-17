package pptx

import (
	"encoding/xml"
	"fmt"
	"path"
	"sort"
	"strings"
)

// reference presentation library OPC XML files use lxml's standalone UTF-8 declaration.
const packageXMLDeclaration = "<?xml version='1.0' encoding='UTF-8' standalone='yes'?>\n"

// Allocate only when an actual relationship is created, independently of shape
// IDs. Allocation order follows authoring: layout, content, then notes.
type relationshipList struct {
	strings.Builder
	count int
}

func (r *relationshipList) add(kind, target string) string {
	r.count++
	id := fmt.Sprintf("rId%d", r.count)
	r.WriteString(rel(id, kind, target))
	return id
}

// Follow reference presentation library _ContentTypesItem: required rels/xml defaults, defaults for
// present recognized extension/type pairs, and sorted per-part overrides.
// This list covers the types emitted by this authoring API; other types retain
// their explicit override rather than being inferred from an extension alone.
func defaultPartType(ext, typ string) bool {
	types := map[string]string{
		"bin":  "application/vnd.openxmlformats-officedocument.presentationml.printerSettings",
		"jpeg": "image/jpeg", "jpg": "image/jpeg", "png": "image/png",
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"xml":  "application/xml",
	}
	return types[strings.ToLower(ext)] == typ
}
func (b *builder) contentTypes() ([]byte, error) {
	var current struct {
		Defaults []struct {
			Extension   string `xml:",attr"`
			ContentType string `xml:",attr"`
		} `xml:"Default"`
		Overrides []struct {
			PartName    string `xml:",attr"`
			ContentType string `xml:",attr"`
		} `xml:"Override"`
	}
	if err := xml.Unmarshal([]byte(b.ct.String()), &current); err != nil {
		return nil, err
	}
	known := map[string]string{}
	for _, d := range current.Defaults {
		known[strings.ToLower(d.Extension)] = d.ContentType
	}
	explicit := map[string]string{}
	for _, o := range current.Overrides {
		explicit[o.PartName] = o.ContentType
	}
	defaults := map[string]string{"rels": "application/vnd.openxmlformats-package.relationships+xml", "xml": "application/xml"}
	overrides := map[string]string{}
	for _, part := range b.p.Parts {
		if strings.HasSuffix(part.Name, ".rels") || part.Name == "[Content_Types].xml" {
			continue
		}
		ext := strings.TrimPrefix(path.Ext(part.Name), ".")
		typ := explicit["/"+part.Name]
		if typ == "" {
			typ = known[strings.ToLower(ext)]
		}
		if typ == "" {
			return nil, fmt.Errorf("missing content type for %s", part.Name)
		}
		if defaultPartType(ext, typ) {
			defaults[ext] = typ
		} else {
			overrides["/"+part.Name] = typ
		}
	}
	var out strings.Builder
	out.WriteString(packageXMLDeclaration + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	keys := func(m map[string]string) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	for _, k := range keys(defaults) {
		fmt.Fprintf(&out, `<Default Extension="%s" ContentType="%s"/>`, esc(k), esc(defaults[k]))
	}
	for _, k := range keys(overrides) {
		fmt.Fprintf(&out, `<Override PartName="%s" ContentType="%s"/>`, esc(k), esc(overrides[k]))
	}
	out.WriteString(`</Types>`)
	return []byte(out.String()), nil
}
