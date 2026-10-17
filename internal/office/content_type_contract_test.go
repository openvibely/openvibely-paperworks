package office

import (
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"reflect"
	"testing"
)

// Pinned document_check.package_report uses case-sensitive suffixes, expanded
// declaration tags and insertion-ordered override keys, not MIME normalization.
func TestContentTypeInspectionReferenceContract(t *testing.T) {
	for _, tc := range []struct {
		name, member, declarations, namespace string
		want                                  []string
	}{
		{"case", "data.bin", `<Default Extension="BIN" ContentType="x"/>`, ` xmlns="urn:types"`, []string{"Missing content type: data.bin"}},
		{"slashes", "data.bin", `<Override PartName="///data.bin" ContentType="x"/>`, ` xmlns="urn:types"`, []string{}},
		{"dotless", "payload", `<Default Extension="payload" ContentType="x"/>`, ` xmlns="urn:types"`, []string{}},
		{"unqualified", "data.bin", `<Default Extension="bin" ContentType="x"/>`, "", []string{"Missing content type: data.bin"}},
		{"override-order", "data.bin", `<Default Extension="bin" ContentType="x"/><Override PartName="/z" ContentType="x"/><Override PartName="/a" ContentType="x"/><Override PartName="/z" ContentType="y"/>`, ` xmlns="urn:types"`, []string{"Content type references missing part: z", "Content type references missing part: a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := referenceCheckPackage(t, []referenceCheckPart{{"[Content_Types].xml", "<Types" + tc.namespace + `><t:Override xmlns:t="urn:types" PartName="/word/document.xml" ContentType="application/xml"/>` + tc.declarations + "</Types>"}, {tc.member, "payload"}, {"word/document.xml", `<w:document xmlns:w="` + NSW + `"/>`}})
			pkg, err := opc.Open(src)
			if err != nil {
				t.Fatal(err)
			}
			got, err := PackageReportOf(pkg)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got["issues"], tc.want) {
				t.Fatalf("got %#v want %#v", got["issues"], tc.want)
			}
		})
	}
}
