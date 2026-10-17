package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPinnedMutationOutputWarnings(t *testing.T) {
	raw, err := os.ReadFile("testdata/mutation-output-warnings.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Name, Policy, Input string
			Overlap             bool
			Parts               []struct{ Name, Data string }
			Warnings, Events    []string
			Accepted            bool
			Outputs             int
			Error               *struct{ Type, Message string }
		}
	}
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Cases) != 192 {
		t.Fatal("incomplete capture")
	}
	for _, c := range ref.Cases {
		t.Run(c.Name+"/"+c.Policy, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(c.Input)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			src := filepath.Join(root, "source.zip")
			out := filepath.Join(root, "out.zip")
			if err = os.WriteFile(src, data, 0600); err != nil {
				t.Fatal(err)
			}
			events, warnings := []string{}, []string{}
			promoted := errors.New("promoted warning")
			result, err := MutateStoredZIPPathWithWarnings(src, func(p *OrderedZIP) error {
				events = append(events, "callback")
				if !c.Overlap {
					p.Set(c.Name+"\x00tail", []byte("second"))
				}
				return nil
			}, func() string { return out }, func(message string) error {
				switch c.Policy {
				case "always":
					warnings = append(warnings, message)
				case "error":
					if c.Error == nil || c.Error.Type != "UserWarning" || c.Error.Message != message {
						t.Fatalf("unexpected promotion %q", message)
					}
					return promoted
				}
				return nil
			})
			if (err == nil) != c.Accepted {
				t.Fatalf("acceptance: %v", err)
			}
			if c.Error != nil && c.Error.Type == "UserWarning" && !errors.Is(err, promoted) {
				t.Fatalf("lost policy error: %v", err)
			}
			if !reflect.DeepEqual(events, c.Events) || !reflect.DeepEqual(warnings, c.Warnings) {
				t.Fatalf("events %v warnings %v; want %v %v", events, warnings, c.Events, c.Warnings)
			}
			_, staterr := os.Stat(out)
			if (staterr == nil) != (c.Outputs == 1) {
				t.Fatalf("output: %v", staterr)
			}
			if !c.Accepted && result != "" {
				t.Fatal("published failure")
			}
			if staterr == nil {
				z, e := zip.OpenReader(out)
				if e != nil {
					t.Fatal(e)
				}
				if len(z.File) != len(c.Parts) {
					t.Fatalf("parts %d want %d", len(z.File), len(c.Parts))
				}
				for i, f := range z.File {
					rd, e := f.Open()
					if e != nil {
						t.Fatal(e)
					}
					data, e := io.ReadAll(rd)
					rd.Close()
					if e != nil {
						t.Fatal(e)
					}
					if f.Name != c.Parts[i].Name || base64.StdEncoding.EncodeToString(data) != c.Parts[i].Data {
						t.Fatal("output member differs")
					}
				}
				z.Close()
			}
			after, e := os.ReadFile(src)
			if e != nil || !bytes.Equal(after, data) {
				t.Fatal("source changed", e)
			}
		})
	}
}
