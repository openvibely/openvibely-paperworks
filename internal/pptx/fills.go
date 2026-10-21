package pptx

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

type PatternFill struct {
	Preset     string `json:"preset"`
	Foreground string `json:"foreground"`
	Background string `json:"background"`
}
type ImageFill struct {
	Path   string       `json:"path"`
	Fit    string       `json:"fit,omitempty"` // stretch (default), cover, or tile
	Anchor *ImageAnchor `json:"anchor,omitempty"`
	Crop   *Crop        `json:"crop,omitempty"`
}

func styleDrawingInPackage(pkg *opc.Package, part string, shape *etree.Element, style *DrawingStyle, base string) error {
	if style == nil {
		return fmt.Errorf("drawing_style required")
	}
	count := 0
	for _, b := range []bool{style.Fill != "", style.Gradient != nil, style.Pattern != nil, style.Image != nil} {
		if b {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("fill, gradient, pattern and image are mutually exclusive")
	}
	if style.Opacity != nil && style.Fill == "" {
		return fmt.Errorf("opacity applies to solid fill; use gradient stop opacity for gradients")
	}
	plain := *style
	plain.Pattern = nil
	plain.Image = nil
	if err := styleDrawing(shape, &plain); err != nil {
		return err
	}
	if style.Pattern == nil && style.Image == nil {
		return nil
	}
	pr := office.Child(shape, pns, "spPr")
	var fill *etree.Element
	if p := style.Pattern; p != nil {
		// ST_PresetPatternVal, ECMA-376 DrawingML.
		presets := " pct5 pct10 pct20 pct25 pct30 pct40 pct50 pct60 pct70 pct75 pct80 pct90 horz vert ltHorz ltVert dkHorz dkVert narHorz narVert dashHorz dashVert cross dnDiag upDiag ltDnDiag ltUpDiag dkDnDiag dkUpDiag wdDnDiag wdUpDiag dashDnDiag dashUpDiag diagCross smCheck lgCheck smGrid lgGrid dotGrid smConfetti lgConfetti horzBrick diagBrick solidDmnd openDmnd dotDmnd plaid sphere weave divot shingle wave trellis zigZag "
		if p.Preset == "" || !strings.Contains(presets, " "+p.Preset+" ") {
			return fmt.Errorf("invalid pattern preset")
		}
		fill = etree.NewElement("a:pattFill")
		fill.CreateAttr("prst", p.Preset)
		if err := rgbaElement(fill.CreateElement("a:fgClr"), p.Foreground, nil); err != nil {
			return err
		}
		if err := rgbaElement(fill.CreateElement("a:bgClr"), p.Background, nil); err != nil {
			return err
		}
	} else {
		i := style.Image
		if i.Path == "" {
			return fmt.Errorf("image fill path required")
		}
		if i.Fit != "" && i.Fit != "stretch" && i.Fit != "cover" && i.Fit != "tile" {
			return fmt.Errorf("image fill fit must be stretch, cover or tile")
		}
		if i.Fit != "cover" && (i.Anchor != nil || i.Crop != nil) {
			return fmt.Errorf("image fill anchor/crop require cover fit")
		}
		source := i.Path
		if !filepath.IsAbs(source) {
			source = filepath.Join(base, source)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if format != "png" && format != "jpeg" {
			return fmt.Errorf("image fill requires PNG or JPEG")
		}
		rect := ""
		if i.Fit == "cover" {
			xf := office.Child(pr, ans, "xfrm")
			if xf == nil {
				return fmt.Errorf("cover fill needs explicit shape dimensions")
			}
			ext := office.Child(xf, ans, "ext")
			if ext == nil {
				return fmt.Errorf("cover fill needs explicit shape dimensions")
			}
			w, e1 := strconv.ParseFloat(ext.SelectAttrValue("cx", ""), 64)
			h, e2 := strconv.ParseFloat(ext.SelectAttrValue("cy", ""), 64)
			if e1 != nil || e2 != nil || !finite(w) || !finite(h) || w <= 0 || h <= 0 {
				return fmt.Errorf("invalid shape dimensions")
			}
			_, rect, err = imagePlacement(Element{W: w, H: h, Fit: "cover", Anchor: i.Anchor, Crop: i.Crop}, cfg.Width, cfg.Height)
			if err != nil {
				return err
			}
		}
		name := unusedPart(pkg, "ppt/media/fill", "."+format)
		pkg.Set(name, data)
		if err := pkg.EnsureContentType(name, "image/"+format); err != nil {
			return err
		}
		target, err := relativePart(part, name)
		if err != nil {
			return err
		}
		id, err := pkg.AddRel(part, rns+"/image", target, false)
		if err != nil {
			return err
		}
		doc := etree.NewDocument()
		if err := doc.ReadFromString("<a:blipFill" + ns + "><a:blip r:embed=\"" + id + "\"/>" + rect + "</a:blipFill>"); err != nil {
			return err
		}
		fill = doc.Root().Copy()
		if i.Fit == "tile" {
			fill.CreateElement("a:tile")
		} else {
			fill.CreateElement("a:stretch").CreateElement("a:fillRect")
		}
	}
	for _, c := range pr.ChildElements() {
		if strings.HasSuffix(c.Tag, "Fill") {
			pr.RemoveChild(c)
		}
	}
	insertDrawingProperty(pr, fill)
	return nil
}
