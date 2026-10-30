package docx

import (
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestHeaderFooterPackageParts(t *testing.T) {
	tests := []struct {
		name         string
		spec         Spec
		wantHeader   bool
		wantFooter   bool
		wantPageNums bool
	}{
		{
			name:       "header only",
			spec:       Spec{Header: "Header only"},
			wantHeader: true,
		},
		{
			name:         "footer only with page numbers",
			spec:         Spec{Footer: "Footer only", PageNumbers: true},
			wantFooter:   true,
			wantPageNums: true,
		},
		{
			name:         "combined with page numbers",
			spec:         Spec{Header: "Combined header", Footer: "Combined footer", PageNumbers: true},
			wantHeader:   true,
			wantFooter:   true,
			wantPageNums: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := Build(tc.spec, "")
			if err != nil {
				t.Fatal(err)
			}
			pkg, err := opc.Read(raw)
			if err != nil {
				t.Fatal(err)
			}

			wantKinds := map[string]bool{
				"header": tc.wantHeader,
				"footer": tc.wantFooter,
			}
			wantTargets := map[string]string{
				"header": "header1.xml",
				"footer": "footer1.xml",
			}
			wantContentTypes := map[string]string{
				"header": "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml",
				"footer": "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml",
			}

			rels, err := pkg.Rels("word/document.xml")
			if err != nil {
				t.Fatal(err)
			}
			relationshipIDs := make(map[string]string)
			relationshipCounts := make(map[string]int)
			for _, rel := range rels {
				for kind, want := range wantKinds {
					if rel.Type != opc.NSOffR+"/"+kind {
						continue
					}
					if !want {
						t.Errorf("unexpected %s relationship: %+v", kind, rel)
						continue
					}
					if rel.Target != wantTargets[kind] {
						t.Errorf("%s relationship target = %q, want %q", kind, rel.Target, wantTargets[kind])
					}
					relationshipIDs[kind] = rel.ID
					relationshipCounts[kind]++
				}
			}

			doc, err := pkg.XML("word/document.xml")
			if err != nil {
				t.Fatal(err)
			}
			sectionReferences := map[string][]string{"header": {}, "footer": {}}
			for _, element := range doc.FindElements("//*") {
				kind := ""
				switch element.Tag {
				case "headerReference":
					kind = "header"
				case "footerReference":
					kind = "footer"
				}
				if kind == "" {
					continue
				}
				for _, attr := range element.Attr {
					if attr.Space == "r" && attr.Key == "id" {
						sectionReferences[kind] = append(sectionReferences[kind], attr.Value)
					}
				}
			}

			contentTypes, err := pkg.XML("[Content_Types].xml")
			if err != nil {
				t.Fatal(err)
			}
			registeredTypes := make(map[string]string)
			for _, element := range contentTypes.Root().ChildElements() {
				if element.Tag == "Override" {
					registeredTypes[element.SelectAttrValue("PartName", "")] = element.SelectAttrValue("ContentType", "")
				}
			}

			for kind, want := range wantKinds {
				partName := "word/" + wantTargets[kind]
				if pkg.Has(partName) != want {
					t.Errorf("part %s present = %t, want %t", partName, pkg.Has(partName), want)
				}
				if want {
					partDoc, err := pkg.XML(partName)
					if err != nil {
						t.Errorf("invalid %s part XML: %v", kind, err)
					} else if partDoc.Root().Tag != map[string]string{"header": "hdr", "footer": "ftr"}[kind] {
						t.Errorf("%s part root = %q", kind, partDoc.Root().Tag)
					}
					if relationshipCounts[kind] != 1 {
						t.Errorf("%s relationship count = %d, want 1", kind, relationshipCounts[kind])
					}
					if relationshipIDs[kind] == "" {
						t.Errorf("missing %s relationship", kind)
					}
					if got := sectionReferences[kind]; len(got) != 1 || got[0] != relationshipIDs[kind] {
						t.Errorf("%s section references = %v, want relationship ID %q", kind, got, relationshipIDs[kind])
					}
					if got := registeredTypes["/"+partName]; got != wantContentTypes[kind] {
						t.Errorf("content type for %s = %q, want %q", partName, got, wantContentTypes[kind])
					}
				} else {
					if relationshipCounts[kind] != 0 {
						t.Errorf("unexpected %s relationships: %d", kind, relationshipCounts[kind])
					}
					if len(sectionReferences[kind]) != 0 {
						t.Errorf("unexpected %s section references: %v", kind, sectionReferences[kind])
					}
				}
			}

			header := string(pkg.Get("word/header1.xml"))
			footer := string(pkg.Get("word/footer1.xml"))
			if tc.wantHeader {
				if !strings.Contains(header, tc.spec.Header) {
					t.Errorf("header part does not contain %q", tc.spec.Header)
				}
				if strings.Contains(header, " PAGE ") || strings.Contains(header, "fldChar") {
					t.Error("header part contains page-number field markup")
				}
			}
			if tc.wantFooter {
				if tc.spec.Footer != "" && !strings.Contains(footer, tc.spec.Footer) {
					t.Errorf("footer part does not contain %q", tc.spec.Footer)
				}
				if strings.Contains(footer, " PAGE ") != tc.wantPageNums {
					t.Errorf("footer page-number field present = %t, want %t", strings.Contains(footer, " PAGE "), tc.wantPageNums)
				}
			}
			if tc.wantHeader && tc.wantFooter {
				if strings.Contains(header, tc.spec.Footer) {
					t.Errorf("header part contains footer content %q", tc.spec.Footer)
				}
				if strings.Contains(footer, tc.spec.Header) {
					t.Errorf("footer part contains header content %q", tc.spec.Header)
				}
			}
		})
	}
}
