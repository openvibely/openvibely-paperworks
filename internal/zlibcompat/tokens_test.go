package zlibcompat

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestPinnedLevel6Tokens(t *testing.T) {
	data, err := os.ReadFile("testdata/level6-tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name      string
			Input     []byte
			InputHash string `json:"input_sha256"`
			TokenHash string `json:"token_sha256"`
			Tokens    int
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 125 {
		t.Fatalf("unexpected reference membership: %d", len(fixture.Cases))
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if fmt.Sprintf("%x", sha256.Sum256(c.Input)) != c.InputHash {
				t.Fatal("input hash")
			}
			before := append([]byte(nil), c.Input...)
			tokens, e := TokenizeLevel6(c.Input)
			if e != nil {
				t.Fatal(e)
			}
			packed := make([]byte, len(tokens)*4)
			rebuilt := make([]byte, 0, len(c.Input))
			for i, token := range tokens {
				binary.LittleEndian.PutUint16(packed[i*4:], token.Value)
				binary.LittleEndian.PutUint16(packed[i*4+2:], token.Distance)
				if token.Distance == 0 {
					if token.Value > 255 {
						t.Fatal("invalid literal", token)
					}
					rebuilt = append(rebuilt, byte(token.Value))
				} else {
					if token.Value < 3 || token.Value > 258 || int(token.Distance) > len(rebuilt) {
						t.Fatal("invalid match", token)
					}
					for n := 0; n < int(token.Value); n++ {
						rebuilt = append(rebuilt, rebuilt[len(rebuilt)-int(token.Distance)])
					}
				}
			}
			if !bytes.Equal(rebuilt, c.Input) || !bytes.Equal(c.Input, before) {
				t.Fatal("reconstruction or input preservation")
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(packed)); got != c.TokenHash || len(tokens) != c.Tokens {
				t.Fatalf("tokens=%d hash=%s; reference tokens=%d hash=%s", len(tokens), got, c.Tokens, c.TokenHash)
			}
		})
	}
}

func TestTokenizationBound(t *testing.T) {
	if tokens, err := TokenizeLevel6(make([]byte, MaxInput+1)); err == nil || tokens != nil {
		t.Fatal("oversize input not rejected")
	}
}
