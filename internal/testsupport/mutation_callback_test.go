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

// The event and archive expectations are from unchanged PresentationNative.mutate.
// Callback-created directory entries must affect output(), but a callback error
// must never invoke it. The callback can replace, delete and append dictionary keys.
func TestReferenceMutationCallbackLifecycle(t *testing.T) {
	for _, mode := range []string{"success", "callback-error", "open-error"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(t.TempDir(), "source.zip")
			parts := &OrderedZIP{}
			parts.Set("a", []byte("old"))
			parts.Set("b", []byte("kept"))
			if err := SaveStoredZIP(source, parts); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(source)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			events := []string{}
			sentinel := errors.New("callback failure")
			out, err := MutateStoredZIP(z, func(p *OrderedZIP) error {
				events = append(events, "callback")
				if !reflect.DeepEqual(p.Parts, parts.Parts) {
					t.Fatal(p.Parts)
				}
				p.Remove("a")
				p.Set("a", []byte("new"))
				p.Set("c", []byte("added"))
				// Exactly one entry, regardless of whether it is a file or directory.
				if e := os.Mkdir(filepath.Join(root, "marker"), 0700); e != nil {
					return e
				}
				if mode == "callback-error" {
					return sentinel
				}
				if mode == "open-error" {
					return os.Mkdir(filepath.Join(root, "2.pptx"), 0700)
				}
				return nil
			}, func() string { events = append(events, "output"); return PresentationOutput(t, root) })
			wantEvents := []string{"callback", "output"}
			if mode == "callback-error" {
				wantEvents = wantEvents[:1]
				if !errors.Is(err, sentinel) {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(events, wantEvents) {
				t.Fatal(events)
			}
			if mode != "success" {
				if err == nil || out != "" {
					t.Fatal(out, err)
				}
			} else {
				if err != nil || out != filepath.Join(root, "1.pptx") {
					t.Fatal(out, err)
				}
				saved, e := zip.OpenReader(out)
				if e != nil {
					t.Fatal(e)
				}
				defer saved.Close()
				got, e := ReadOrderedZIP(saved)
				if e != nil {
					t.Fatal(e)
				}
				want := []ZIPPart{{"b", "kept"}, {"a", "new"}, {"c", "added"}}
				if !reflect.DeepEqual(got.Parts, want) {
					t.Fatal(got.Parts)
				}
			}
			entries, e := os.ReadDir(root)
			if e != nil {
				t.Fatal(e)
			}
			wantCount := 2
			if mode == "callback-error" {
				wantCount = 1
			}
			if len(entries) != wantCount {
				t.Fatal(entries)
			}
			after, e := os.ReadFile(source)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", e)
			}
		})
	}
}
