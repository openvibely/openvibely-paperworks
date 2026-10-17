package office

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"os"
	"reflect"
	"testing"
)

// Shared with conversion callers of DocumentGapChecks.rewrite.
type referenceOrderedZIP struct{ *testsupport.OrderedZIP }

func (p *referenceOrderedZIP) set(name string, data []byte) { p.Set(name, data) }
func (p *referenceOrderedZIP) remove(name string)           { p.Remove(name) }
func referenceEditedZIP(z *zip.ReadCloser, transform func(*referenceOrderedZIP) error) ([]byte, error) {
	return testsupport.EditStoredZIP(z, func(p *testsupport.OrderedZIP) error { return transform(&referenceOrderedZIP{p}) })
}
func referenceRewriteZIP(path string, transform func(*referenceOrderedZIP) error) error {
	return testsupport.RewriteStoredZIP(path, func(p *testsupport.OrderedZIP) error { return transform(&referenceOrderedZIP{p}) })
}
func TestReferenceOrderedRewriteContract(t *testing.T) {
	p := referenceCheckPackage(t, []referenceCheckPart{{"z", "old"}, {"a", "a"}, {"z", "last"}, {"directory/", ""}})
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = referenceRewriteZIP(p, func(p *referenceOrderedZIP) error { p.remove("z"); return fmt.Errorf("callback failed") }); err == nil {
		t.Fatal("failure swallowed")
	}
	after, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("callback failure changed source", err)
	}
	err = referenceRewriteZIP(p, func(p *referenceOrderedZIP) error {
		if p.Parts[0].Name != "z" || p.Parts[0].Data != "last" {
			return fmt.Errorf("duplicate semantics: %v", p.Parts)
		}
		p.set("a", []byte("updated"))
		p.remove("z")
		p.set("z", []byte("reinserted"))
		p.set("new", []byte("new"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var names, values []string
	for _, f := range z.File {
		names = append(names, f.Name)
		values = append(values, string(referencePreviewZIPRead(t, z, f.Name)))
		if f.Method != zip.Store {
			t.Fatal("compression", f.Method)
		}
	}
	if !reflect.DeepEqual(names, []string{"a", "directory/", "z", "new"}) || !reflect.DeepEqual(values, []string{"updated", "", "reinserted", "new"}) {
		t.Fatal(names, values)
	}
}
