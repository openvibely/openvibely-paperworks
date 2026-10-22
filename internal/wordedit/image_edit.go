package wordedit

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path"
	"strconv"
)

const nsWP = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
const nsA = "http://schemas.openxmlformats.org/drawingml/2006/main"
const nsPic = "http://schemas.openxmlformats.org/drawingml/2006/picture"

func imageEdit(d *Doc, part string, root *etree.Element, s map[string]any) error {
	op := str(s, "operation")
	var drawing *etree.Element
	if op != "image-insert" {
		var images []*etree.Element
		for _, candidate := range all(root, "drawing") {
			// Only a standalone picture is an image target. Charts, shapes and
			// grouped artwork also use w:drawing and must not be deleted/resized.
			graphics := office.All(candidate, nsA, "graphicData")
			if len(graphics) == 1 && graphics[0].SelectAttrValue("uri", "") == nsPic &&
				len(office.Children(graphics[0], nsPic, "pic")) == 1 {
				images = append(images, candidate)
			}
		}
		i, ok := intArg(s["image"])
		if !ok || i < 0 || i >= len(images) {
			return fmt.Errorf("image index out of range")
		}
		drawing = images[i]
	}
	if op == "image-delete" {
		remove(drawing)
		d.Touch(part)
		return nil
	}
	if op == "image-insert" || op == "image-replace" {
		file := str(s, "path")
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		config, format, err := image.DecodeConfig(f)
		f.Close()
		if err != nil {
			return err
		}
		if format != "png" && format != "jpeg" {
			return fmt.Errorf("image must be PNG or JPEG")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if len(data) > 32<<20 {
			return fmt.Errorf("image exceeds 32 MiB")
		}
		name := d.Pkg.UnusedName("word/media/edit", "."+format)
		d.Pkg.Set(name, data)
		if err = d.Pkg.EnsureContentType(name, "image/"+format); err != nil {
			return err
		}
		target := path.Base(name)
		if path.Dir(part) == "word" {
			target = "media/" + target
		} else {
			return fmt.Errorf("image edits require a standard word/ document or story part")
		}
		rid, err := d.Pkg.AddRel(part, relBase+"image", target, false)
		if err != nil {
			return err
		}
		if op == "image-replace" {
			blips := office.All(drawing, nsA, "blip")
			if len(blips) != 1 {
				return fmt.Errorf("drawing must contain exactly one image")
			}
			blips[0].CreateAttr("r:embed", rid)
			office.RemoveSVGImageSource(blips[0])
		} else {
			container := root
			if part == d.Main {
				container = d.Body()
			}
			paras := office.Children(container, nsW, "p")
			i, ok := intArg(s["paragraph"])
			if !ok || i < 0 || i >= len(paras) {
				return fmt.Errorf("paragraph out of range")
			}
			width := float64(config.Width) * 9525
			height := float64(config.Height) * 9525
			if w, ok := s["width_in"].(float64); ok {
				if w <= 0 || w > 100 {
					return fmt.Errorf("invalid width_in")
				}
				width = w * 914400
				height = width * float64(config.Height) / float64(config.Width)
			}
			id := 1
			for _, p := range d.WordParts() {
				r, _ := d.Root(p)
				for _, pr := range office.All(r, nsWP, "docPr") {
					v, _ := strconv.Atoi(pr.SelectAttrValue("id", "0"))
					id = max(id, v+1)
				}
			}
			run := W("r")
			drawing = run.CreateElement("w:drawing")
			inline := drawing.CreateElement("wp:inline")
			inline.CreateAttr("xmlns:wp", nsWP)
			inline.CreateAttr("xmlns:a", nsA)
			inline.CreateAttr("xmlns:pic", nsPic)
			inline.CreateAttr("xmlns:r", nsR)
			extent := inline.CreateElement("wp:extent")
			extent.CreateAttr("cx", strconv.FormatInt(int64(width), 10))
			extent.CreateAttr("cy", strconv.FormatInt(int64(height), 10))
			pr := inline.CreateElement("wp:docPr")
			pr.CreateAttr("id", strconv.Itoa(id))
			pr.CreateAttr("name", fmt.Sprintf("Picture %d", id))
			pr.CreateAttr("descr", str(s, "description"))
			g := inline.CreateElement("a:graphic").CreateElement("a:graphicData")
			g.CreateAttr("uri", nsPic)
			pic := g.CreateElement("pic:pic")
			nv := pic.CreateElement("pic:nvPicPr")
			pictureProperties := nv.CreateElement("pic:cNvPr")
			pictureProperties.CreateAttr("id", "0")
			pictureProperties.CreateAttr("name", fmt.Sprintf("Picture %d", id))
			nv.CreateElement("pic:cNvPicPr")
			fill := pic.CreateElement("pic:blipFill")
			fill.CreateElement("a:blip").CreateAttr("r:embed", rid)
			fill.CreateElement("a:stretch").CreateElement("a:fillRect")
			sp := pic.CreateElement("pic:spPr")
			x := sp.CreateElement("a:xfrm")
			off := x.CreateElement("a:off")
			off.CreateAttr("x", "0")
			off.CreateAttr("y", "0")
			ext := x.CreateElement("a:ext")
			ext.CreateAttr("cx", strconv.FormatInt(int64(width), 10))
			ext.CreateAttr("cy", strconv.FormatInt(int64(height), 10))
			sp.CreateElement("a:prstGeom").CreateAttr("prst", "rect")
			paras[i].AddChild(run)
		}
	}
	if op == "image-resize" || op == "image-insert" || op == "image-replace" {
		for _, v := range []struct{ key, attr string }{{"width_in", "cx"}, {"height_in", "cy"}} {
			if raw, ok := s[v.key]; ok {
				n, ok := raw.(float64)
				if !ok || n <= 0 || n > 100 {
					return fmt.Errorf("invalid %s", v.key)
				}
				value := strconv.FormatInt(int64(n*914400), 10)
				for _, ext := range office.All(drawing, nsWP, "extent") {
					ext.CreateAttr(v.attr, value)
				}
				for _, ext := range office.All(drawing, nsA, "ext") {
					ext.CreateAttr(v.attr, value)
				}
			}
		}
	}
	d.Touch(part)
	return nil
}
