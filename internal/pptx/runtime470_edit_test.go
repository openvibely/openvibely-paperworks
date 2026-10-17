package pptx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// Exact payload hashes from unchanged runtime461-470, including its expected
// notes-master verification failure. These do not assert whole-PPTX equivalence.
func TestRuntime470EditedPayloads(t *testing.T) {
	var fixture struct {
		Parts map[string]struct {
			Input  string `json:"input_sha256"`
			Output string `json:"reference_output_sha256"`
		}
	}
	b, err := os.ReadFile("testdata/runtime470-edit.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	input := testsupport.OriginalPackageFixture(t, "runtime-workflow-fixtures.zip", "chart-formula", ".pptx")
	p, err := opc.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range fixture.Parts {
		if got := fmt.Sprintf("%x", sha256.Sum256(p.Get(name))); got != want.Input {
			t.Fatalf("%s input %s", name, got)
		}
	}
	output := filepath.Join(t.TempDir(), "final.pptx")
	_, err = EditFile(context.Background(), input, output, EditSpec{Edits: []Edit{
		{Slide: 2, ShapeID: "2", Operation: "run-text", Text: "Revenue totaled $80"},
		{Slide: 2, Operation: "notes", Text: "Revenue: $80; Q1; USD. Only listed revenue streams; no profitability inference."},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = opc.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range fixture.Parts {
		if got := fmt.Sprintf("%x", sha256.Sum256(p.Get(name))); got != want.Output {
			t.Errorf("%s output %s want %s", name, got, want.Output)
		}
	}
	if _, err = office.VerifyPreservation(input, output, []string{"ppt/slides/slide2.xml", "ppt/notesSlides/notesSlide2.xml"}); err != nil {
		t.Fatal(err)
	}
}

func TestReplacementRunNamespaceScope(t *testing.T) {
	for _, tc := range []struct {
		name, xml string
		local     bool
	}{
		{"inherited", `<root xmlns:a="` + ans + `"><a:p><a:r><a:t>old</a:t></a:r></a:p></root>`, false},
		{"paragraph", `<root><a:p xmlns:a="` + ans + `"><a:r><a:t>old</a:t></a:r></a:p></root>`, false},
		{"different-prefix", `<root xmlns:d="` + ans + `"><d:p><d:r><d:t>old</d:t></d:r></d:p></root>`, true},
		{"shadowed", `<root xmlns:a="` + ans + `" xmlns:d="` + ans + `"><d:p xmlns:a="urn:other"><d:r><d:t>old</d:t></d:r></d:p></root>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := etree.NewDocument()
			if err := d.ReadFromString(tc.xml); err != nil {
				t.Fatal(err)
			}
			p := d.Root().ChildElements()[0]
			replaceParagraph(p, "new")
			r := p.ChildElements()[0]
			if r.NamespaceURI() != ans || r.ChildElements()[0].NamespaceURI() != ans {
				t.Fatal("incorrect DrawingML namespace")
			}
			if got := r.SelectAttr("xmlns:a") != nil; got != tc.local {
				t.Fatalf("local declaration=%v want %v", got, tc.local)
			}
			serialized, err := d.WriteToString()
			if err != nil {
				t.Fatal(err)
			}
			reread := etree.NewDocument()
			if err = reread.ReadFromString(serialized); err != nil {
				t.Fatal(err)
			}
			if reread.Root().ChildElements()[0].ChildElements()[0].NamespaceURI() != ans {
				t.Fatal("namespace lost on serialization")
			}
		})
	}
}
