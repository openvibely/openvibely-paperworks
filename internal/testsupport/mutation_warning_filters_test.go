package testsupport

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestPinnedMutationOrderedFilters(t *testing.T) {
	type event struct {
		Module  string
		Line    int
		Message string
	}
	var fixture struct {
		Cases []struct {
			Name  string
			Steps []struct {
				Filter  *MutationWarningFilter
				Append  bool
				Module  string
				Line    int
				Message string
			}
			Emitted []event
			Errors  []int
		}
	}
	raw, err := os.ReadFile("testdata/mutation-ordered-filters.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			r := new(MutationWarningRegistry)
			emitted := []event{}
			promoted := []int{}
			for i, s := range c.Steps {
				if s.Filter != nil {
					if err := r.AddFilter(*s.Filter, s.Append); err != nil {
						t.Fatal(err)
					}
					continue
				}
				err := r.Handler(MutationWarningLocation{s.Module, s.Line}, func(message string) error { emitted = append(emitted, event{s.Module, s.Line, message}); return nil })(s.Message)
				if err != nil {
					var warning *MutationWarningError
					if !errors.As(err, &warning) || warning.Message != s.Message {
						t.Fatal(err)
					}
					promoted = append(promoted, i)
				}
			}
			if !reflect.DeepEqual(emitted, c.Emitted) || !reflect.DeepEqual(promoted, c.Errors) {
				t.Fatalf("emitted %v want %v; promoted %v want %v", emitted, c.Emitted, promoted, c.Errors)
			}
		})
	}
}

func TestMutationInvalidFilterPreservesSuppression(t *testing.T) {
	r := new(MutationWarningRegistry)
	deliveries := 0
	handler := r.Handler(MutationWarningLocation{"zipfile", 1}, func(string) error { deliveries++; return nil })
	if err := handler("a"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []MutationWarningFilter{{Action: "invalid"}, {Action: "always", Line: -1}, {Action: "always", Message: "["}, {Action: "always", Module: "["}, {Action: "always", Category: "CustomWarning"}} {
		if err := r.AddFilter(f, false); err == nil {
			t.Fatal("invalid policy accepted")
		}
		if err := handler("a"); err != nil {
			t.Fatal(err)
		}
	}
	if deliveries != 1 {
		t.Fatalf("invalid filters reset suppression: %d", deliveries)
	}
}
