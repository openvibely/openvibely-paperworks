package office

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestReferenceMutateAppendsNewMember(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.zip")
	f, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, value := range []string{"first", "last"} {
		p, e := w.Create("same.txt")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.Write([]byte(value)); e != nil {
			t.Fatal(e)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	output := referencePreviewMutatedZIP(t, z, "new.txt", []byte("new"))
	result, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	if len(result.File) != 2 || result.File[0].Name != "same.txt" || result.File[1].Name != "new.txt" {
		t.Fatal("ordered dictionary mutation differs")
	}
	if string(referencePreviewZIPRead(t, result, "same.txt")) != "last" || string(referencePreviewZIPRead(t, result, "new.txt")) != "new" {
		t.Fatal("payload differs")
	}
	for _, p := range result.File {
		if p.Method != zip.Store {
			t.Fatal("expected stored member")
		}
	}
}
