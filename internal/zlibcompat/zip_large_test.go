package zlibcompat

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestPinnedLargeZIPStreams(t *testing.T) {
	raw, err := os.ReadFile("testdata/zip-large.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Name            string
			Input, Expected []byte
		}
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := DeflateZIPLevel6(c.Input)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, c.Expected) {
				t.Fatalf("stream mismatch: got %d bytes want %d", len(got), len(c.Expected))
			}
		})
	}
}
