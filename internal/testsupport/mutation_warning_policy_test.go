package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPinnedMutationWarningPolicies(t *testing.T) {
	raw, err := os.ReadFile("testdata/mutation-warning-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Name, Policy, Input string
			Warnings, Events    []string
			Accepted            bool
			Outputs             int
			Error               *struct{ Type, Message string }
		}
	}
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Cases) != 108 {
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
			result, err := MutateStoredZIPPathWithWarnings(src, func(p *OrderedZIP) error { events = append(events, "callback"); return nil }, func() string { return out }, func(message string) error {
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
			after, e := os.ReadFile(src)
			if e != nil || !bytes.Equal(after, data) {
				t.Fatal("source changed", e)
			}
		})
	}
}

// Warning errors follow the same ownership boundary as other acquisition errors:
// close the owned source even on promotion, and retain close-error precedence.
func TestMutationWarningClosePrecedence(t *testing.T) {
	raw, err := os.ReadFile("testdata/mutation-warning-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Policy, Input string
			Error         *struct{ Type string }
		}
	}
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	var data []byte
	for _, c := range ref.Cases {
		if c.Policy == "error" && c.Error != nil && c.Error.Type == "UserWarning" {
			data, err = base64.StdEncoding.DecodeString(c.Input)
			break
		}
	}
	if err != nil || len(data) == 0 {
		t.Fatal("missing warning input", err)
	}
	for _, closeFails := range []bool{false, true} {
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		events := []string{}
		promotion := errors.New("promotion")
		closing := errors.New("close")
		_, err = mutateOrderedZIPWithWarnings(z, func() error {
			events = append(events, "close")
			if closeFails {
				return closing
			}
			return nil
		}, func(*OrderedZIP) error { t.Fatal("callback after warning failure"); return nil }, func() string { t.Fatal("allocation after warning failure"); return "" }, func(string) error { events = append(events, "warning"); return promotion })
		want := promotion
		if closeFails {
			want = closing
		}
		if !errors.Is(err, want) || !reflect.DeepEqual(events, []string{"warning", "close"}) {
			t.Fatal(err, events)
		}
	}
}
