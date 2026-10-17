package zlibcompat

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"testing"
)

func TestPinnedLevel6Streams(t *testing.T) {
	for _, file := range []string{"testdata/level6-streams.json"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var f struct {
			Cases []struct {
				Name       string
				Input      []byte
				InputHash  string `json:"input_sha256"`
				StreamHash string `json:"stream_sha256"`
			}
		}
		if err = json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		if len(f.Cases) != 209 {
			t.Fatalf("reference membership: %d", len(f.Cases))
		}
		for _, c := range f.Cases {
			t.Run(c.Name, func(t *testing.T) {
				if fmt.Sprintf("%x", sha256.Sum256(c.Input)) != c.InputHash {
					t.Fatal("input hash")
				}
				before := bytes.Clone(c.Input)
				stream, err := DeflateLevel6(c.Input)
				if err != nil {
					t.Fatal(err)
				}
				if got := fmt.Sprintf("%x", sha256.Sum256(stream)); got != c.StreamHash {
					t.Fatalf("stream hash %s want %s", got, c.StreamHash)
				}
				decoder := flate.NewReader(bytes.NewReader(stream))
				decoded, err := io.ReadAll(decoder)
				decoder.Close()
				if err != nil || !bytes.Equal(decoded, c.Input) || !bytes.Equal(before, c.Input) {
					t.Fatal("round-trip/input preservation", err)
				}
			})
		}
	}
}

func TestDeflateInputBound(t *testing.T) {
	if out, err := DeflateLevel6(make([]byte, MaxInput+1)); err == nil || out != nil {
		t.Fatal("oversize input accepted")
	}
}

// High entropy reaches zlib's 16383-token buffer boundary before MaxInput.
// Multi-block compression remains explicitly unsupported, not silently altered.
func TestDeflateTokenBlockBound(t *testing.T) {
	rng := rand.New(rand.NewPCG(470, 643))
	data := make([]byte, MaxInput)
	for i := range data {
		data[i] = byte(rng.Uint32())
	}
	tokens, err := TokenizeLevel6(data)
	if err != nil || len(tokens) < 16383 {
		t.Fatal("invalid bound probe", err, len(tokens))
	}
	if out, err := DeflateLevel6(data); err == nil || out != nil {
		t.Fatal("multi-block input accepted")
	}
}
