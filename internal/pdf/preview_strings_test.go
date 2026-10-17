package pdf

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestPreviewPinnedStringEncoding(t *testing.T) {
	raw, err := os.ReadFile("testdata/preview-writer/strings.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct{ Raw, Output, Kind string }
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.Raw, func(t *testing.T) {
			data, err := hex.DecodeString(c.Raw)
			if err != nil {
				t.Fatal(err)
			}
			escaped, err := types.Escape(string(data))
			if err != nil {
				t.Fatal(err)
			}
			for _, o := range []types.Object{types.NewHexLiteral(data), types.StringLiteral(*escaped)} {
				var out bytes.Buffer
				if err = writePreviewObject(&out, o); err != nil {
					t.Fatal(err)
				}
				if out.String() != c.Output {
					t.Fatalf("%T %s: got %q want %q", o, c.Kind, out.String(), c.Output)
				}
			}
		})
	}
}
