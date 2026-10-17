package office

import (
	"archive/zip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReferencePackageHelperOrderAndStore(t *testing.T) {
	parts := []referenceCheckPart{{"z.xml", "first"}, {"a.xml", "second"}, {"directory/", ""}, {"opaque.bin", "\x00\xff"}}
	p := referenceCheckPackage(t, parts)
	z, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != len(parts) {
		t.Fatal("member count", len(z.File))
	}
	for i, f := range z.File {
		if f.Name != parts[i].name || f.Method != zip.Store {
			t.Fatalf("member %d: %s method %d", i, f.Name, f.Method)
		}
		if string(referencePreviewZIPRead(t, z, f.Name)) != parts[i].data {
			t.Fatal("payload", f.Name)
		}
	}
}

func TestReferencePresentationOutputContract(t *testing.T) {
	dir := t.TempDir()
	var got []string
	output := func() { got = append(got, filepath.Base(referencePresentationOutput(t, dir))) }
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	output()
	output() // does not reserve a filename
	write("unrelated.txt")
	output()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	output()
	write("2.pptx")
	output()
	if err := os.Remove(filepath.Join(dir, "unrelated.txt")); err != nil {
		t.Fatal(err)
	}
	output() // deliberately selects the already existing 2.pptx
	if !reflect.DeepEqual(got, []string{"0.pptx", "0.pptx", "1.pptx", "2.pptx", "3.pptx", "2.pptx"}) {
		t.Fatal(got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatal("output reserved a path", entries, err)
	}
}
