package pptx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const attachmentPart = "ppt/slides/slide1.xml"

func attachmentFixture(t *testing.T, replacement bool) (*opc.Package, *etree.Document) {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	p.Set("[Content_Types].xml", []byte(`<Types xmlns="`+opc.NSCT+`"><Default Extension="xml" ContentType="application/xml"/></Types>`))
	p.Set(opc.RelsName(attachmentPart), []byte(`<Relationships xmlns="`+opc.NSRel+`"><Relationship Id="rId3" Type="keep" Target="../media/old.svg"/><Relationship Id="rId1" Type="keep" Target="https://example.com" TargetMode="External"/></Relationships>`))
	xml := `<p:sp` + ns + `><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="100" cy="100"/></a:xfrm><a:solidFill><a:srgbClr val="123456"/></a:solidFill></p:spPr></p:sp>`
	if replacement {
		xml = `<p:pic` + ns + ` xmlns:asvg="http://schemas.microsoft.com/office/drawing/2016/SVG/main"><p:blipFill><a:blip r:embed="rId3" r:link="rId1"><a:extLst><a:ext uri="svg"><asvg:svgBlip r:embed="rId3"/></a:ext><a:ext uri="keep"><a:keep/></a:ext></a:extLst></a:blip><a:srcRect l="10000" t="0" r="20000" b="0"/><a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr><a:xfrm><a:off x="10" y="20"/><a:ext cx="100" cy="200"/></a:xfrm></p:spPr></p:pic>`
	}
	d, err := opc.ParseXML([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	return p, d
}

func attachmentInput(t *testing.T, format string) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	var err error
	if format == "png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	// The decoded format, not the source extension, determines the media suffix.
	path := filepath.Join(t.TempDir(), "image.bin")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return b.Bytes(), path
}

func TestImageAttachmentWorkflows(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		for _, workflow := range []string{"replacement", "stretch", "tile", "cover"} {
			t.Run(format+"/"+workflow, func(t *testing.T) {
				replacement := workflow == "replacement"
				prefix := "ppt/media/fill"
				if replacement {
					prefix = "ppt/media/replacement"
				}
				p, d := attachmentFixture(t, replacement)
				data, path := attachmentInput(t, format)
				for _, stem := range []string{"fill", "replacement"} {
					for _, n := range []int{1, 3} {
						p.Set(fmt.Sprintf("ppt/media/%s%d.%s", stem, n, format), []byte("occupied"))
					}
				}
				before := map[string][]byte{}
				for _, part := range p.Parts {
					before[part.Name] = bytes.Clone(part.Data)
				}
				names := p.Names()
				rels, err := p.Rels(attachmentPart)
				if err != nil {
					t.Fatal(err)
				}
				original, _ := d.WriteToString()
				for i, n := range []int{2, 4} {
					id := []string{"rId2", "rId4"}[i]
					if replacement {
						err = editObject(p, attachmentPart, d.Root(), Edit{Operation: "replace-image", Path: path})
					} else {
						fill := &ImageFill{Path: path, Fit: workflow}
						if workflow == "cover" {
							fill.Anchor = &ImageAnchor{X: 0, Y: 0}
							fill.Crop = &Crop{Left: 0.1, Right: 0.1}
						}
						err = styleDrawingInPackage(p, attachmentPart, d.Root(), &DrawingStyle{Image: fill}, "")
					}
					if err != nil {
						t.Fatal(err)
					}
					name := fmt.Sprintf("%s%d.%s", prefix, n, format)
					names = append(names, name)
					if !reflect.DeepEqual(p.Names(), names) {
						t.Fatalf("part order: %v", p.Names())
					}
					if !bytes.Equal(p.Get(name), data) || p.ContentType(name) != "image/"+format {
						t.Fatalf("media bytes/type changed: %s", name)
					}
					rels = append(rels, opc.Relationship{ID: id, Type: rns + "/image", Target: "../media/" + filepath.Base(name)})
					got, err := p.Rels(attachmentPart)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, rels) {
						t.Fatalf("relationships: %#v, want %#v", got, rels)
					}
					if replacement {
						want := strings.Replace(original, `r:embed="rId3" r:link="rId1"`, `r:embed="`+id+`"`, 1)
						want = strings.Replace(want, `<a:ext uri="svg"><asvg:svgBlip r:embed="rId3"/></a:ext>`, "", 1)
						actual, _ := d.WriteToString()
						if actual != want {
							t.Fatalf("replacement XML:\n%s\nwant:\n%s", actual, want)
						}
					} else {
						rect := ""
						if workflow == "cover" {
							rect = `<a:srcRect l="10000" t="0" r="40000" b="0"/>`
						}
						mode := `<a:stretch><a:fillRect/></a:stretch>`
						if workflow == "tile" {
							mode = `<a:tile/>`
						}
						expected, err := opc.ParseXML([]byte(`<a:blipFill` + ns + `><a:blip r:embed="` + id + `"/>` + rect + mode + `</a:blipFill>`))
						if err != nil {
							t.Fatal(err)
						}
						actual := etree.NewDocument()
						actual.SetRoot(d.FindElement("//a:blipFill").Copy())
						a, _ := actual.WriteToString()
						b, _ := expected.WriteToString()
						if a != b {
							t.Fatalf("fill XML:\n%s\nwant:\n%s", a, b)
						}
						if d.FindElement("//a:solidFill") != nil {
							t.Fatal("old fill retained")
						}
					}
				}
				for name, data := range before {
					if name == "[Content_Types].xml" || name == opc.RelsName(attachmentPart) {
						continue
					}
					if !bytes.Equal(p.Get(name), data) {
						t.Fatalf("changed existing part %s", name)
					}
				}
				ct, err := p.XML("[Content_Types].xml")
				if err != nil {
					t.Fatal(err)
				}
				overrides := ct.Root().SelectElements("Override")
				if len(overrides) != 2 {
					t.Fatalf("content types: %d overrides", len(overrides))
				}
				for i, n := range []int{2, 4} {
					if overrides[i].SelectAttrValue("PartName", "") != fmt.Sprintf("/%s%d.%s", prefix, n, format) || overrides[i].SelectAttrValue("ContentType", "") != "image/"+format {
						t.Fatal("content type registration/order changed")
					}
				}
			})
		}
	}
}

func TestImageAttachmentFailures(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		for _, failure := range []string{"corrupt", "unreadable", "content-types", "relationships"} {
			t.Run(fmt.Sprintf("replacement=%t/%s", replacement, failure), func(t *testing.T) {
				p, d := attachmentFixture(t, replacement)
				data, path := attachmentInput(t, "png")
				switch failure {
				case "corrupt":
					if err := os.WriteFile(path, []byte("not an image"), 0600); err != nil {
						t.Fatal(err)
					}
				case "unreadable":
					path = filepath.Join(t.TempDir(), "missing.png")
				case "content-types":
					p.Set("[Content_Types].xml", []byte("<broken"))
				case "relationships":
					p.Set(opc.RelsName(attachmentPart), []byte("<broken"))
				}
				before := map[string][]byte{}
				for _, part := range p.Parts {
					before[part.Name] = bytes.Clone(part.Data)
				}
				names := p.Names()
				var err error
				if replacement {
					err = editObject(p, attachmentPart, d.Root(), Edit{Operation: "replace-image", Path: path})
				} else {
					err = styleDrawingInPackage(p, attachmentPart, d.Root(), &DrawingStyle{Image: &ImageFill{Path: path}}, "")
				}
				if err == nil {
					t.Fatal("expected failure")
				}
				prefix := "fill"
				if replacement {
					prefix = "replacement"
				}
				name := "ppt/media/" + prefix + "1.png"
				if failure == "content-types" || failure == "relationships" {
					names = append(names, name)
					if !bytes.Equal(p.Get(name), data) {
						t.Fatal("Set must precede content type registration")
					}
					if failure == "relationships" && p.ContentType(name) != "image/png" {
						t.Fatal("content type must precede AddRel")
					}
				}
				if !reflect.DeepEqual(names, p.Names()) {
					t.Fatalf("unexpected attached parts: %v", p.Names())
				}
				for name, data := range before {
					if failure == "relationships" && name == "[Content_Types].xml" {
						continue
					}
					if !bytes.Equal(p.Get(name), data) {
						t.Fatalf("unexpected mutation: %s", name)
					}
				}
			})
		}
	}
}
