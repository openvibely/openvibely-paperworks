package equation

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"reflect"
	"testing"
)

// Pinned Sharp output: exact inflated IDAT and density, independently of
// compressed bytes. Expectations are captured from unchanged equation.cjs.
func TestPinnedPNGScanlines(t *testing.T) {
	for _, tc := range []struct {
		name, tex string
		inline    bool
		hash      string
	}{
		{"continued", `\cfrac{1}{1+\cfrac{1}{x}}`, false, "35196d4d49adf462362fcfd066a5009df8f6edf67d9f4c6aac4da411ae45f884"},
		{"inline", `\cfrac{a}{b}`, true, "f009cf104b56683a20365aa240d9817eaf69e388e58d7b48f7da5c0b082eee94"},
		{"color", `\color{red}{\cfrac{a}{b}}`, false, "eba5a9f6e6d733fd91ad7d1ff5fb23a2b12abf82c5f4827c752fa21c0ad2a07a"},
		{"left", `\cfrac[l]{a}{b+c}`, false, "c7aae0dae29b634099dcc241f2cb4a9e38d540ec436873bcf27e584a9568b957"},
		{"deep", `x^{\cfrac{a}{b}}`, false, "0660991a544b8f8c124ee213b141712c3e51310067e6b99b604ce9540fe9dd9f"},
		{"vphantom-script", `x^{\vphantom{\frac{a}{b}}+y}`, false, "074b77ec4a87d21a69aa29d158ff61a58115fc272d1d08493eb96b00b0b13c87"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Defaults()
			o.Inline = tc.inline
			raw, err := PNG(context.Background(), tc.tex, o)
			if err != nil {
				t.Fatal(err)
			}
			var idat []byte
			var kinds []string
			for pos := 8; pos < len(raw); {
				if len(raw)-pos < 12 {
					t.Fatal("truncated chunk")
				}
				n := int(binary.BigEndian.Uint32(raw[pos : pos+4]))
				if n > len(raw)-pos-12 {
					t.Fatal("invalid length")
				}
				kind := string(raw[pos+4 : pos+8])
				payload := raw[pos+8 : pos+8+n]
				if crc32.ChecksumIEEE(raw[pos+4:pos+8+n]) != binary.BigEndian.Uint32(raw[pos+8+n:pos+12+n]) {
					t.Fatal("CRC mismatch")
				}
				kinds = append(kinds, kind)
				switch kind {
				case "IHDR":
					if len(payload) != 13 || !bytes.Equal(payload[8:], []byte{8, 6, 0, 0, 0}) {
						t.Fatal("not eight-bit RGBA")
					}
				case "pHYs":
					if !bytes.Equal(payload, []byte{0, 0, 46, 35, 0, 0, 46, 35, 1}) {
						t.Fatalf("density %x", payload)
					}
				case "IDAT":
					idat = append(idat, payload...)
				}
				pos += 12 + n
			}
			if !reflect.DeepEqual(kinds, []string{"IHDR", "pHYs", "IDAT", "IEND"}) {
				t.Fatal(kinds)
			}
			zr, err := zlib.NewReader(bytes.NewReader(idat))
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(zr)
			zr.Close()
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != tc.hash {
				t.Fatalf("inflated IDAT %s; want %s", got, tc.hash)
			}
		})
	}
}
