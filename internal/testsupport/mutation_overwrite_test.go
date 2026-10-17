package testsupport

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Compared with unchanged PresentationNative.mutate: callback precedes opening
// an existing destination; successful truncation retains its inode and mode.
func TestMutationExistingDestination(t *testing.T) {
	for _, name := range []string{"truncate", "read-only", "callback-error"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			src, dst := filepath.Join(root, "source.zip"), filepath.Join(root, "2.pptx")
			if err := SaveStoredZIP(src, &OrderedZIP{Parts: []ZIPPart{{"original", "keep"}}}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			sentinel := bytes.Repeat([]byte("x"), 10000)
			if err := os.WriteFile(dst, sentinel, 0600); err != nil {
				t.Fatal(err)
			}
			mode := os.FileMode(0640)
			if name == "read-only" {
				mode = 0400
			}
			if err := os.Chmod(dst, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dst, 0600) })
			st, err := os.Stat(dst)
			if err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(src)
			if err != nil {
				t.Fatal(err)
			}
			p, err := ReadOrderedZIP(z)
			ce := z.Close()
			if err != nil || ce != nil {
				t.Fatal(err, ce)
			}
			calls := 0
			callbackErr := errors.New("callback sentinel")
			callback := func() error {
				calls++
				if !reflect.DeepEqual(p.Parts, []ZIPPart{{"original", "keep"}}) {
					t.Fatal(p.Parts)
				}
				p.Set("added", []byte("value"))
				if name == "callback-error" {
					return callbackErr
				}
				return nil
			}
			err = callback()
			if err == nil {
				out := PresentationOutput(t, root)
				if out != dst {
					t.Fatal(out)
				}
				err = SaveStoredZIP(out, p)
			}
			switch name {
			case "truncate":
				if err != nil {
					t.Fatal(err)
				}
			case "read-only":
				if !errors.Is(err, os.ErrPermission) {
					t.Fatalf("permission error: %v", err)
				}
			case "callback-error":
				if !errors.Is(err, callbackErr) {
					t.Fatal(err)
				}
			}
			if calls != 1 {
				t.Fatal(calls)
			}
			after, err := os.ReadFile(src)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
			end, err := os.Stat(dst)
			if err != nil || !os.SameFile(st, end) || end.Mode().Perm() != mode {
				t.Fatal("inode/mode changed", err)
			}
			raw, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if name != "truncate" {
				if !bytes.Equal(raw, sentinel) {
					t.Fatal("failed mutation changed destination")
				}
				return
			}
			if len(raw) >= len(sentinel) {
				t.Fatal("stale trailing bytes")
			}
			saved, err := zip.OpenReader(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer saved.Close()
			got, err := ReadOrderedZIP(saved)
			if err != nil || !reflect.DeepEqual(got.Parts, []ZIPPart{{"original", "keep"}, {"added", "value"}}) {
				t.Fatal(got, err)
			}
			for _, f := range saved.File {
				if f.Method != zip.Store {
					t.Fatal(f.Method)
				}
			}
		})
	}
}
