package testsupport

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestPinnedMutationWarningRegistry(t *testing.T) {
	type event struct {
		Module  string
		Line    int
		Message string
		Reset   bool `json:"reset,omitempty"`
	}
	var fixture struct {
		Cases []struct {
			Action          string
			Events, Emitted []event
			Errors          []int
		}
	}
	raw, err := os.ReadFile("testdata/mutation-warning-registry.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.Action, func(t *testing.T) {
			r := new(MutationWarningRegistry)
			if err := r.SetAction(c.Action); err != nil {
				t.Fatal(err)
			}
			emitted := []event{}
			promoted := []int{}
			for i, e := range c.Events {
				if e.Reset {
					if err := r.SetAction(c.Action); err != nil {
						t.Fatal(err)
					}
				}
				h := r.Handler(MutationWarningLocation{e.Module, e.Line}, func(s string) error {
					emitted = append(emitted, event{Module: e.Module, Line: e.Line, Message: s})
					return nil
				})
				if err := h(e.Message); err != nil {
					var warning *MutationWarningError
					if !errors.As(err, &warning) || warning.Message != e.Message {
						t.Fatal(err)
					}
					promoted = append(promoted, i)
				}
			}
			if !reflect.DeepEqual(emitted, c.Emitted) || !reflect.DeepEqual(promoted, c.Errors) {
				t.Fatalf("emitted %v want %v; errors %v want %v", emitted, c.Emitted, promoted, c.Errors)
			}
		})
	}
}
