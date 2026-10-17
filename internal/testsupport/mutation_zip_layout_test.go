package testsupport

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Cases were executed through the unchanged PresentationNative.mutate helper.
// Inputs and expected callback contents are captured reference data, not output
// synthesized by the native decoder. Reference exception classes remain distinct.
func TestReferenceMutationZIPLayout(t *testing.T) {
	raw, err := os.ReadFile("testdata/mutation-zip-layout.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Cases []struct {
			Name        string
			Input       string
			InputSHA256 string `json:"input_sha256"`
			Accepted    bool
			Events      []string
			Entries     []string
			Parts       []struct{ Name, Data string }
		}
	}
	if err := json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference.Cases) != 32 {
		t.Fatal("incomplete reference capture", len(reference.Cases))
	}
	for _, c := range reference.Cases {
		t.Run(c.Name, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(c.Input)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%x", sha256.Sum256(data)) != c.InputSHA256 {
				t.Fatal("reference input changed")
			}
			root := t.TempDir()
			source := filepath.Join(root, "input.zip")
			outdir := filepath.Join(root, "out")
			if err = os.Mkdir(outdir, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(source, data, 0600); err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { z.Close() })
			events := []string{}
			want := []ZIPPart{}
			for _, p := range c.Parts {
				b, e := base64.StdEncoding.DecodeString(p.Data)
				if e != nil {
					t.Fatal(e)
				}
				want = append(want, ZIPPart{p.Name, string(b)})
			}
			output, err := MutateStoredZIP(z, func(p *OrderedZIP) error {
				events = append(events, "callback")
				if !reflect.DeepEqual(p.Parts, want) {
					t.Fatalf("callback got %#v want %#v", p.Parts, want)
				}
				p.Set("added", []byte("value"))
				return nil
			}, func() string { events = append(events, "output"); return PresentationOutput(t, outdir) })
			if (err == nil) != c.Accepted {
				t.Fatalf("accepted=%v want %v: %v", err == nil, c.Accepted, err)
			}
			if !reflect.DeepEqual(events, c.Events) {
				t.Fatal(events, c.Events)
			}
			after, e := os.ReadFile(source)
			if e != nil || !bytes.Equal(data, after) {
				t.Fatal("source changed", e)
			}
			entries, e := os.ReadDir(outdir)
			if e != nil {
				t.Fatal(e)
			}
			names := []string{}
			for _, e := range entries {
				names = append(names, e.Name())
			}
			if !reflect.DeepEqual(names, c.Entries) {
				t.Fatal(names, c.Entries)
			}
			if !c.Accepted {
				if output != "" {
					t.Fatal("published after failure", output)
				}
				return
			}
			saved, e := zip.OpenReader(output)
			if e != nil {
				t.Fatal(e)
			}
			defer saved.Close()
			got, e := ReadOrderedZIP(saved)
			if e != nil {
				t.Fatal(e)
			}
			want = append(want, ZIPPart{"added", "value"})
			if !reflect.DeepEqual(got.Parts, want) {
				t.Fatal(got.Parts, want)
			}
			for _, f := range saved.File {
				if f.Method != zip.Store {
					t.Fatal("mutation output method changed", f.Method)
				}
			}
		})
	}
}
