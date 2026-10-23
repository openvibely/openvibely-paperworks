package pptx

import (
	"bytes"
	"fmt"
	"github.com/beevik/etree"
	"image"
	"math"
	"os"
	"strconv"
	"strings"
)

func loadSlideImage(path, svg string) ([]byte, image.Config, string, error) {
	var data []byte
	var err error
	if svg != "" {
		if path != "" {
			return nil, image.Config{}, "", fmt.Errorf("path and svg are mutually exclusive")
		}
		data = []byte(svg)
	} else {
		info, e := os.Stat(path)
		if e != nil {
			return nil, image.Config{}, "", e
		}
		if !info.Mode().IsRegular() || info.Size() > 32<<20 {
			return nil, image.Config{}, "", fmt.Errorf("image must be a regular file <=32 MiB")
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, image.Config{}, "", err
		}
	}
	if len(data) > 32<<20 {
		return nil, image.Config{}, "", fmt.Errorf("image exceeds 32 MiB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil && bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) {
		cfg, err = svgConfig(data)
		format = "svg"
	}
	if err != nil {
		return nil, cfg, "", err
	}
	if format != "png" && format != "jpeg" && format != "svg" {
		return nil, cfg, "", fmt.Errorf("image must be PNG, JPEG or SVG")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 100000000 {
		return nil, cfg, "", fmt.Errorf("image dimensions exceed limits")
	}
	return data, cfg, format, nil
}

func svgConfig(data []byte) (image.Config, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return image.Config{}, err
	}
	root := doc.Root()
	if root == nil || root.Tag != "svg" || root.NamespaceURI() != "http://www.w3.org/2000/svg" {
		return image.Config{}, fmt.Errorf("expected SVG root with namespace")
	}
	length := func(s string) float64 {
		s = strings.TrimSpace(s)
		scale := 1.0
		for _, u := range []struct {
			suffix string
			scale  float64
		}{{"px", 1}, {"pt", 96.0 / 72}, {"pc", 16}, {"mm", 96.0 / 25.4}, {"cm", 96.0 / 2.54}, {"in", 96}} {
			if strings.HasSuffix(s, u.suffix) {
				s = strings.TrimSuffix(s, u.suffix)
				scale = u.scale
				break
			}
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || !finite(n) || n <= 0 {
			return 0
		}
		return n * scale
	}
	w, h := length(root.SelectAttrValue("width", "")), length(root.SelectAttrValue("height", ""))
	if w == 0 || h == 0 {
		fields := strings.Fields(strings.ReplaceAll(root.SelectAttrValue("viewBox", ""), ",", " "))
		if len(fields) != 4 {
			return image.Config{}, fmt.Errorf("SVG requires positive dimensions or viewBox")
		}
		vals := make([]float64, 4)
		for i, s := range fields {
			n, err := strconv.ParseFloat(s, 64)
			if err != nil || !finite(n) {
				return image.Config{}, fmt.Errorf("invalid SVG viewBox")
			}
			vals[i] = n
		}
		if vals[2] <= 0 || vals[3] <= 0 {
			return image.Config{}, fmt.Errorf("invalid SVG viewBox")
		}
		if w == 0 && h == 0 {
			w, h = vals[2], vals[3]
		} else if w == 0 {
			w = h * vals[2] / vals[3]
		} else {
			h = w * vals[3] / vals[2]
		}
	}
	if !finite(w) || !finite(h) || w > 100000 || h > 100000 {
		return image.Config{}, fmt.Errorf("SVG dimensions exceed limits")
	}
	return image.Config{Width: int(math.Max(1, math.Round(w))), Height: int(math.Max(1, math.Round(h)))}, nil
}
