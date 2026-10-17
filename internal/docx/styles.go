package docx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// overlayStyles applies a sparse, explicit author resource to the complete
// template. Unmentioned definitions and their order are retained. This does not
// resolve external resources or merge properties inside a selected definition.
func overlayStyles(template, overlay []byte) ([]byte, error) {
	base, err := opc.ParseXML(template)
	if err != nil {
		return nil, err
	}
	src, err := opc.ParseXML(overlay)
	if err != nil {
		return nil, err
	}
	root := base.Root()
	for _, a := range src.Root().Attr {
		key := a.Key
		if a.Space != "" {
			key = a.Space + ":" + key
		}
		if existing := root.SelectAttr(key); existing != nil {
			if existing.Value != a.Value {
				return nil, fmt.Errorf("styles overlay: conflicting root attribute %s", key)
			}
		} else {
			root.CreateAttr(key, a.Value)
		}
	}
	key := func(e *etree.Element) (string, error) {
		if e.NamespaceURI() != NSW {
			return "", fmt.Errorf("styles overlay: unexpected namespace on %s", e.Tag)
		}
		switch e.Tag {
		case "docDefaults", "latentStyles":
			return e.Tag, nil
		case "style":
			for i := range e.Attr {
				a := &e.Attr[i]
				if a.Key == "styleId" && a.NamespaceURI() == NSW && a.Value != "" {
					return "style:" + a.Value, nil
				}
			}
			return "", fmt.Errorf("styles overlay: style requires a nonempty styleId")
		default:
			return "", fmt.Errorf("styles overlay: unsupported element %s", e.Tag)
		}
	}
	seen := map[string]bool{}
	for _, e := range src.Root().ChildElements() {
		id, err := key(e)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, fmt.Errorf("styles overlay: duplicate %s", id)
		}
		seen[id] = true
		var old *etree.Element
		for _, child := range root.ChildElements() {
			k, _ := key(child)
			if k == id {
				old = child
				break
			}
		}
		if old != nil {
			root.InsertChild(old, e.Copy())
			root.RemoveChild(old)
		} else {
			root.AddChild(e.Copy())
		}
	}
	return base.WriteToBytes()
}
