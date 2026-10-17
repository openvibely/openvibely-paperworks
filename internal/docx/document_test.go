package docx

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// Complete payload hashes from the pinned reference DOCX library authoring path. Checks
// namespace/section context, whitespace, table serialization and explicit pages.
func TestReferenceDocumentXML(t *testing.T) {
	raw, err := os.ReadFile("testdata/document-xml.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Installed string `json:"installed_document_sha256"`
		Cases     []struct {
			Name string
			Spec Spec
			Hash string `json:"document_sha256"`
		}
	}
	if err := json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, spec Spec, base, want string) {
		t.Helper()
		raw, err := Build(spec, base)
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Read(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(p.Get("word/document.xml"))); got != want {
			t.Fatalf("document.xml: got %s want %s\n%s", got, want, p.Get("word/document.xml"))
		}
	}
	t.Run("installed", func(t *testing.T) {
		base := "../../testdata/document-examples/document_word/examples"
		raw, err := os.ReadFile(base + "/report.json")
		if err != nil {
			t.Fatal(err)
		}
		var spec Spec
		if err = json.Unmarshal(raw, &spec); err != nil {
			t.Fatal(err)
		}
		check(t, spec, base, oracle.Installed)
	})
	for _, c := range oracle.Cases {
		t.Run(c.Name, func(t *testing.T) { check(t, c.Spec, "", c.Hash) })
	}
}
