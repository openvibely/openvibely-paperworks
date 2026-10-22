package docx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestStyleResourcesAndAutofit(t *testing.T) {
	root := t.TempDir()
	styles := []byte(`<x:styles xmlns:x="` + NSW + `"><x:style x:type="paragraph" x:styleId="Title"><x:rPr><x:sz x:val="52"/></x:rPr></x:style></x:styles>`)
	theme := []byte(`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Custom"><a:themeElements/></a:theme>`)
	for n, b := range map[string][]byte{"styles.xml": styles, "theme.xml": theme} {
		if err := os.WriteFile(filepath.Join(root, n), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, autofit := range []bool{false, true} {
		raw, err := Build(Spec{StylesFile: "styles.xml", ThemeFile: "theme.xml", Blocks: []Block{{Type: "table", Rows: [][]string{{"a", "b"}}, Widths: []int{1000, 2000}, Autofit: autofit}}}, root)
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Read(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p.Get("word/styles.xml"), styles) || !bytes.Equal(p.Get("word/theme/theme1.xml"), theme) {
			t.Fatal("author resources rewritten")
		}
		if !strings.Contains(string(p.Get("word/_rels/document.xml.rels")), `Target="theme/theme1.xml"`) || !strings.Contains(string(p.Get("[Content_Types].xml")), "application/vnd.openxmlformats-officedocument.theme+xml") {
			t.Fatal("unlinked theme")
		}
		doc := string(p.Get("word/document.xml"))
		if strings.Contains(doc, `w:type="auto"`) != autofit || strings.Contains(doc, `w:type="fixed"`) == autofit {
			t.Fatal("table layout mode", doc)
		}
		if !strings.Contains(doc, `<w:gridCol w:w="1000"/>`) || !strings.Contains(doc, `<w:gridCol w:w="2000"/>`) {
			t.Fatal("grid widths lost")
		}
	}
}

func TestAuthorResourcesAcceptAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	styles := []byte(`<w:styles xmlns:w="` + NSW + `"><w:style w:type="paragraph" w:styleId="Title"/></w:styles>`)
	theme := []byte(`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Absolute"><a:themeElements/></a:theme>`)
	stylesPath := filepath.Join(root, "styles.xml")
	themePath := filepath.Join(root, "theme.xml")
	if err := os.WriteFile(stylesPath, styles, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(themePath, theme, 0600); err != nil {
		t.Fatal(err)
	}

	raw, err := Build(Spec{StylesFile: stylesPath, ThemeFile: themePath}, root)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Read(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pkg.Get("word/styles.xml"), styles) || !bytes.Equal(pkg.Get("word/theme/theme1.xml"), theme) {
		t.Fatal("absolute-path author resources were rewritten")
	}
}

func TestInvalidAuthorResourceDoesNotPublish(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		theme     bool
	}{
		{"missing", "", false}, {"malformed", "<x", false}, {"wrong-namespace", `<styles/>`, false},
		{"wrong-root", `<w:document xmlns:w="` + NSW + `"/>`, false},
		{"theme-root", `<a:styles xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"/>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "resource.xml")
			if tc.raw != "" {
				if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
					t.Fatal(err)
				}
			}
			spec := Spec{StylesFile: path}
			if tc.theme {
				spec.StylesFile = ""
				spec.ThemeFile = path
			}
			out := filepath.Join(root, "out.docx")
			if err := Create(spec, root, out); err == nil {
				t.Fatal("invalid resource accepted")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("output published", err)
			}
		})
	}
}

func TestCompatibilityMode(t *testing.T) {
	for _, mode := range []int{0, 14, 15, 13} {
		raw, err := Build(Spec{CompatibilityMode: mode}, "")
		if mode == 13 {
			if err == nil {
				t.Fatal("unsupported mode accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Read(raw)
		if err != nil {
			t.Fatal(err)
		}
		want := `w:val="15"`
		if mode == 14 {
			want = `w:val="14"`
		}
		if !strings.Contains(string(p.Get("word/settings.xml")), want) {
			t.Fatal("wrong compatibility mode")
		}
	}
}
