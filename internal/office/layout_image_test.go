package office

import (
	"github.com/beevik/etree"
	"testing"
)

func TestLayoutImageSettings(t *testing.T) {
	d := etree.NewDocument()
	if err := d.ReadFromString(`<p:pic xmlns:p="` + NSP + `" xmlns:a="` + NSA + `"><p:blipFill><a:srcRect l="12500" b="-5000"/><a:stretch><a:fillRect r="25000"/></a:stretch></p:blipFill><p:spPr><a:prstGeom prst="ellipse"/></p:spPr></p:pic>`); err != nil {
		t.Fatal(err)
	}
	r := map[string]any{}
	layoutImage(r, d.Root(), Child(d.Root(), NSP, "blipFill"))
	if r["crop"].(map[string]float64)["l"] != .125 || r["crop"].(map[string]float64)["b"] != -.05 {
		t.Fatal(r)
	}
	if r["fit"].(map[string]any)["fill_rect"].(map[string]float64)["r"] != .25 || r["mask"].(map[string]any)["preset"] != "ellipse" {
		t.Fatal(r)
	}
}
