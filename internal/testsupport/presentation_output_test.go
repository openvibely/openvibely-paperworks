package testsupport

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPresentationOutputEntryPolicy(t *testing.T) {
	root := t.TempDir()
	next := func() string { return filepath.Base(PresentationOutput(t, root)) }
	got := []string{next(), next()}
	if err := os.Mkdir(filepath.Join(root, "unrelated"), 0700); err != nil {
		t.Fatal(err)
	}
	got = append(got, next())
	if err := os.WriteFile(filepath.Join(root, "1.pptx"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	got = append(got, next())
	if err := os.Remove(filepath.Join(root, "unrelated")); err != nil {
		t.Fatal(err)
	}
	got = append(got, next())
	if !reflect.DeepEqual(got, []string{"0.pptx", "0.pptx", "1.pptx", "2.pptx", "1.pptx"}) {
		t.Fatal(got)
	}
}
