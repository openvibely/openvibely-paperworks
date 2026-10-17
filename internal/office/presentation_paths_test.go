package office

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

var referencePresentationRootsByTest sync.Map

// A Reference unittest owns one temporary directory, including all its subtests.
// Register from the parent so cleanup cannot run between loop iterations.
func referencePresentationRoot(t *testing.T) string {
	t.Helper()
	key := strings.SplitN(t.Name(), "/", 2)[0]
	if root, ok := referencePresentationRootsByTest.Load(key); ok {
		return root.(string)
	}
	root := t.TempDir()
	referencePresentationRootsByTest.Store(key, root)
	t.Cleanup(func() { referencePresentationRootsByTest.Delete(key) })
	return root
}
func referenceNextPresentation(t *testing.T) string {
	t.Helper()
	return referencePresentationOutput(t, referencePresentationRoot(t))
}

// Allocate only after the original ordered callback succeeds.
func referenceMutatePresentation(t *testing.T, original *zip.ReadCloser, transform func(*referenceOrderedZIP) error) (string, error) {
	t.Helper()
	return testsupport.MutateStoredZIP(original, func(parts *testsupport.OrderedZIP) error {
		return transform(&referenceOrderedZIP{parts})
	}, func() string { return referenceNextPresentation(t) })
}

// Save the already-applied original mutation at the same ordered Store boundary.
func referenceWriteMutation(t *testing.T, p *opc.Package) string {
	t.Helper()
	parts := &testsupport.OrderedZIP{}
	for _, name := range p.Names() {
		parts.Set(name, p.Get(name))
	}
	out := referenceNextPresentation(t)
	if err := testsupport.SaveStoredZIP(out, parts); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReferencePresentationSharedOutputLifecycle(t *testing.T) {
	root := referencePresentationRoot(t)
	for i, want := range []string{"0.pptx", "1.pptx"} {
		t.Run(want, func(t *testing.T) {
			out := referenceNextPresentation(t)
			if filepath.Dir(out) != root || filepath.Base(out) != want {
				t.Fatal(out)
			}
			if again := referenceNextPresentation(t); again != out {
				t.Fatal("output reserved", again, out)
			}
			if err := os.WriteFile(out, []byte{byte(i)}, 0600); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := os.Remove(filepath.Join(root, "0.pptx")); err != nil {
		t.Fatal(err)
	}
	if got := referenceNextPresentation(t); got != filepath.Join(root, "1.pptx") {
		t.Fatal("count must include extant output, without uniqueness scan", got)
	}
}

func TestReferenceMutationCallbackPublication(t *testing.T) {
	for _, mode := range []string{"replace", "reinsert", "failure"} {
		t.Run(mode, func(t *testing.T) {
			// Independent unittest roots for these supplemental oracle cases.
			root := referencePresentationRoot(t)
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if err := os.Remove(filepath.Join(root, entry.Name())); err != nil {
					t.Fatal(err)
				}
			}
			fixture := referenceCheckPackage(t, []referenceCheckPart{{name: "z", data: "old"}, {name: "a", data: "a"}, {name: "z", data: "last"}, {name: "directory/", data: ""}})
			before, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "source.zip")
			if err := os.WriteFile(source, before, 0600); err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(source)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			out, err := referenceMutatePresentation(t, z, func(p *referenceOrderedZIP) error {
				want := []testsupport.ZIPPart{{Name: "z", Data: "last"}, {Name: "a", Data: "a"}, {Name: "directory/", Data: ""}}
				if !reflect.DeepEqual(p.Parts, want) {
					t.Fatalf("callback acquisition: %v", p.Parts)
				}
				if mode != "replace" {
					p.remove("z")
				}
				p.set("z", []byte("new"))
				if mode == "failure" {
					return fmt.Errorf("callback failed")
				}
				return nil
			})
			after, readErr := os.ReadFile(source)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", readErr)
			}
			if mode == "failure" {
				entries, e := os.ReadDir(root)
				if err == nil || out != "" || e != nil || len(entries) != 1 {
					t.Fatal("published after failure", out, err, entries, e)
				}
				return
			}
			if err != nil || filepath.Base(out) != "1.pptx" {
				t.Fatal(out, err)
			}
			saved, err := zip.OpenReader(out)
			if err != nil {
				t.Fatal(err)
			}
			defer saved.Close()
			var got []testsupport.ZIPPart
			for _, f := range saved.File {
				if f.Method != zip.Store {
					t.Fatal("not Store", f.Name)
				}
				got = append(got, testsupport.ZIPPart{Name: f.Name, Data: string(referencePreviewZIPRead(t, saved, f.Name))})
			}
			want := []testsupport.ZIPPart{{Name: "z", Data: "new"}, {Name: "a", Data: "a"}, {Name: "directory/", Data: ""}}
			if mode == "reinsert" {
				want = []testsupport.ZIPPart{{Name: "a", Data: "a"}, {Name: "directory/", Data: ""}, {Name: "z", Data: "new"}}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal(got, want)
			}
		})
	}
}
