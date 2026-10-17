package pdf

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestPinnedReportStream(t *testing.T) {
	raw, err := os.ReadFile("testdata/reportlab-streams.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name          string
			Input         []byte
			EncodedSHA256 string `json:"encoded_sha256"`
			Size          int
		}
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			sd, err := reportStream(c.Input)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(sd.Raw)); got != c.EncodedSHA256 || len(sd.Raw) != c.Size {
				t.Fatalf("encoded %s (%d bytes), want %s (%d)", got, len(sd.Raw), c.EncodedSHA256, c.Size)
			}
			if err = sd.Decode(); err != nil {
				t.Fatal(err)
			}
			if string(sd.Content) != string(c.Input) {
				t.Fatal("decoded bytes changed")
			}
		})
	}
}
