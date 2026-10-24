package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// URI is a URL for external links, or an exact inspected slide part for internal links.
// Empty URI/action clears the link. Navigation actions do not need a URI.
type TextLink struct {
	URI        string `json:"uri,omitempty"`
	IsExternal bool   `json:"is_external,omitempty"`
	Action     string `json:"action,omitempty"`
}

func setTextLink(pkg *opc.Package, part string, pr *etree.Element, link *TextLink) error {
	if link.IsExternal {
		if link.Action != "" {
			return fmt.Errorf("external links cannot specify a navigation action")
		}
		if err := setHyperlink(pkg, part, pr, link.URI); err != nil {
			return err
		}
		for _, h := range office.Children(pr, ans, "hlinkClick") {
			h.CreateAttr("xmlns:r", rns)
		}
		return nil
	}
	action := link.Action
	id := ""
	if link.URI != "" {
		if pkg.ContentType(link.URI) != "application/vnd.openxmlformats-officedocument.presentationml.slide+xml" || !pkg.Has(link.URI) {
			return fmt.Errorf("internal link uri must be an existing slide part")
		}
		if action != "" && action != "ppaction://hlinksldjump" {
			return fmt.Errorf("slide target requires hlinksldjump action")
		}
		action = "ppaction://hlinksldjump"
		var err error
		id, err = pkg.AddRel(part, rns+"/slide", opc.Relative(part, link.URI), false)
		if err != nil {
			return err
		}
	} else if action != "" {
		switch action {
		case "ppaction://hlinkshowjump?jump=nextslide", "ppaction://hlinkshowjump?jump=previousslide", "ppaction://hlinkshowjump?jump=firstslide", "ppaction://hlinkshowjump?jump=lastslide", "ppaction://hlinkshowjump?jump=endshow", "ppaction://hlinkshowjump?jump=lastslideviewed":
		default:
			return fmt.Errorf("unsupported presentation navigation action")
		}
	}
	for _, h := range office.Children(pr, ans, "hlinkClick") {
		pr.RemoveChild(h)
	}
	if id == "" && action == "" {
		return nil
	}
	h := etree.NewElement("a:hlinkClick")
	h.CreateAttr("xmlns:r", rns)
	h.CreateAttr("r:id", id)
	h.CreateAttr("action", action)
	if ext := office.Child(pr, ans, "extLst"); ext != nil {
		pr.InsertChildAt(ext.Index(), h)
	} else {
		pr.AddChild(h)
	}
	return nil
}
