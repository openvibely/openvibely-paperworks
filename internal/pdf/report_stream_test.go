package pdf

import (
	"bytes"
	"crypto/sha256"
	"encoding/ascii85"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/adler32"
	"os"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/zlibcompat"
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

func TestReportStreamSegmentedEncoding(t *testing.T) {
	content := []byte("BT /F1 12 Tf (Small report content) Tj ET\n")
	large := bytes.Repeat([]byte("BT /F1 10 Tf (Repeated report paragraph) Tj ET\n"), 1<<14)
	poorlyCompressible := poorlyCompressibleReportData(1 << 20)
	for _, tc := range []struct {
		name  string
		input []byte
	}{
		{name: "empty"},
		{name: "small", input: content},
		{name: "large", input: large},
		{name: "poorly-compressible", input: poorlyCompressible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reportStream(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			want := legacyReportStreamEncoding(t, tc.input)
			if !bytes.Equal(got.Raw, want) {
				t.Fatalf("segmented output differs from contiguous ASCII85 encoding (%d vs %d bytes)", len(got.Raw), len(want))
			}
			if err := got.Decode(); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Content, tc.input) {
				t.Fatal("decoded bytes changed")
			}
		})
	}
}

func legacyReportStreamEncoding(t *testing.T, content []byte) []byte {
	t.Helper()
	raw, err := zlibcompat.DeflateZIPLevel6(content)
	if err != nil {
		t.Fatal(err)
	}
	z := make([]byte, 0, len(raw)+6)
	z = append(z, 0x78, 0x9c)
	z = append(z, raw...)
	z = binary.BigEndian.AppendUint32(z, adler32.Checksum(content))
	encoded := make([]byte, ascii85.MaxEncodedLen(len(z))+2)
	n := ascii85.Encode(encoded, z)
	return append(encoded[:n], '~', '>')
}

func BenchmarkReportStream(b *testing.B) {
	cases := []struct {
		name  string
		input func() []byte
	}{
		{name: "small", input: func() []byte { return bytes.Repeat([]byte("BT /F1 12 Tf (Quarterly report) Tj ET\n"), 12) }},
		{name: "large", input: func() []byte {
			return bytes.Repeat([]byte("BT /F1 10 Tf (Revenue by region and quarter) Tj ET\n"), 1<<16)
		}},
		{name: "large-poorly-compressible", input: func() []byte { return poorlyCompressibleReportData(4 << 20) }},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			input := tc.input()
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sd, err := reportStream(input)
				if err != nil {
					b.Fatal(err)
				}
				reportStreamBenchmarkSink = sd.Raw
			}
		})
	}
}

var reportStreamBenchmarkSink []byte

func poorlyCompressibleReportData(size int) []byte {
	data := make([]byte, size)
	state := uint32(0x6d2b79f5)
	for i := range data {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		data[i] = byte(state)
	}
	return data
}
