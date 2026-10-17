package testsupport

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Unchanged PresentationNative.mutate closes its source context before edit(),
// and closes on acquisition errors too. The oracle observes the actual Reference
// source file, not merely the position of a callback in a synthetic sequence.
func TestReferenceMutationSourceLifetime(t *testing.T) {
	for _, mode := range []string{"success", "acquisition-error", "callback-error", "open-error"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(t.TempDir(), "source.zip")
			parts := &OrderedZIP{Parts: []ZIPPart{{"a", "old"}, {"b", "kept"}}}
			raw, err := StoredZIPBytes(parts)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "acquisition-error" {
				raw[31] ^= 1
			}
			if err = os.WriteFile(source, raw, 0600); err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { z.Close() })
			closed := func() {
				t.Helper()
				stream, e := z.File[0].Open()
				if e == nil {
					_, e = io.ReadAll(stream)
					stream.Close()
				}
				if !errors.Is(e, os.ErrClosed) {
					t.Fatalf("source still readable or wrong error: %v", e)
				}
			}
			events := []string{}
			sentinel := errors.New("callback failure")
			out, err := MutateStoredZIP(z, func(p *OrderedZIP) error {
				closed()
				events = append(events, "callback")
				if !reflect.DeepEqual(parts.Parts, p.Parts) {
					t.Fatal(p.Parts)
				}
				if e := os.Mkdir(filepath.Join(root, "marker"), 0700); e != nil {
					return e
				}
				p.Set("c", []byte("added"))
				if mode == "callback-error" {
					return sentinel
				}
				if mode == "open-error" {
					return os.Mkdir(filepath.Join(root, "2.pptx"), 0700)
				}
				return nil
			}, func() string { closed(); events = append(events, "output"); return PresentationOutput(t, root) })
			closed()
			want := []string{"callback", "output"}
			entriesWant := []string{"1.pptx", "marker"}
			switch mode {
			case "acquisition-error":
				want = []string{}
				entriesWant = []string{}
				if !errors.Is(err, zip.ErrChecksum) {
					t.Fatal(err)
				}
			case "callback-error":
				want = []string{"callback"}
				entriesWant = []string{"marker"}
				if !errors.Is(err, sentinel) {
					t.Fatal(err)
				}
			case "open-error":
				entriesWant = []string{"2.pptx", "marker"}
				if err == nil {
					t.Fatal("destination directory accepted")
				}
			case "success":
				if err != nil || filepath.Base(out) != "1.pptx" {
					t.Fatal(out, err)
				}
				saved, e := zip.OpenReader(out)
				if e != nil {
					t.Fatal(e)
				}
				defer saved.Close()
				got, e := ReadOrderedZIP(saved)
				if e != nil || !reflect.DeepEqual(got.Parts, []ZIPPart{{"a", "old"}, {"b", "kept"}, {"c", "added"}}) {
					t.Fatal(got, e)
				}
			}
			if mode != "success" && out != "" {
				t.Fatal("output published after failure", out)
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatal(events, want)
			}
			after, e := os.ReadFile(source)
			if e != nil || !bytes.Equal(after, raw) {
				t.Fatal("source changed", e)
			}
			entries, e := os.ReadDir(root)
			if e != nil {
				t.Fatal(e)
			}
			names := []string{}
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if !reflect.DeepEqual(names, entriesWant) {
				t.Fatal(names, entriesWant)
			}
		})
	}
}
