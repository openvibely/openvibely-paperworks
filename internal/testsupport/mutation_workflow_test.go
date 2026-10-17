package testsupport

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Captured from unchanged PresentationNative.mutate across one shared directory.
// In particular output() counts entries, not unused names: deleting an earlier
// output can make a later successful mutation overwrite its own source.
func TestPinnedMutationWorkflow(t *testing.T) {
	raw, err := os.ReadFile("testdata/mutation-workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Steps []struct {
			Action, Output  string
			Error           *string
			Events, Entries []string
			SourcePreserved bool `json:"source_preserved"`
			Parts           []struct{ Name, Data string }
		}
	}
	if err = json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	if len(oracle.Steps) != 8 {
		t.Fatal("incomplete workflow")
	}
	root := t.TempDir()
	source := filepath.Join(root, "input.zip")
	if err = SaveStoredZIP(source, &OrderedZIP{Parts: []ZIPPart{{"a", "old"}, {"b", "kept"}, {"folder/", ""}}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range oracle.Steps {
		t.Run(step.Action, func(t *testing.T) {
			before, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			events := []string{}
			out, e := MutateStoredZIPPath(source, func(p *OrderedZIP) error {
				events = append(events, "callback")
				switch step.Action {
				case "replace":
					p.Set("a", []byte("new"))
				case "reinsert":
					p.Remove("a")
					p.Set("a", []byte("last"))
				case "callback-error":
					p.Parts = nil
					if err := os.Mkdir(filepath.Join(root, "marker"), 0700); err != nil {
						return err
					}
					return errors.New("callback failure")
				case "add":
					p.Set("λ", []byte("unicode"))
					p.Set("folder/", []byte("payload"))
				case "clear":
					p.Parts = nil
				case "empty-name":
					p.Set("", []byte("empty key"))
				case "delete-output":
					if err := os.Remove(filepath.Join(root, "1.pptx")); err != nil {
						return err
					}
					p.Set("next", []byte("value"))
				case "collision":
					p.Set("collision", []byte("last"))
				default:
					t.Fatal("unrecognized action", step.Action)
				}
				return nil
			}, func() string { events = append(events, "output"); return PresentationOutput(t, root) })
			if (e != nil) != (step.Error != nil) {
				t.Fatalf("outcome %v expected %v", e, step.Error)
			}
			if !reflect.DeepEqual(events, step.Events) {
				t.Fatal(events, step.Events)
			}
			after, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			if bytes.Equal(before, after) != step.SourcePreserved {
				t.Fatal("source preservation differs")
			}
			if step.Output == "" {
				if out != "" {
					t.Fatal(out)
				}
			} else {
				if out != filepath.Join(root, step.Output) {
					t.Fatal(out, step.Output)
				}
				z, f, e := openMutationZIP(out)
				if e != nil {
					t.Fatal(e)
				}
				got, e := readOrderedZIP(z)
				closeErr := f.Close()
				if e != nil || closeErr != nil {
					t.Fatal(e, closeErr)
				}
				want := &OrderedZIP{}
				for _, p := range step.Parts {
					data, e := hex.DecodeString(p.Data)
					if e != nil {
						t.Fatal(e)
					}
					want.Set(p.Name, data)
				}
				if !reflect.DeepEqual(got.Parts, want.Parts) && !(len(got.Parts) == 0 && len(want.Parts) == 0) {
					t.Fatalf("parts %#v want %#v", got, want)
				}
				source = out
			}
			entries, e := os.ReadDir(root)
			if e != nil {
				t.Fatal(e)
			}
			names := []string{}
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if !reflect.DeepEqual(names, step.Entries) {
				t.Fatal(names, step.Entries)
			}
		})
		if t.Failed() {
			t.FailNow()
		}
	}
}
