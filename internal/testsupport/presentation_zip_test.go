package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestReferencePresentationZIPMetadata(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Name           string
			Data           []byte
			CRC            uint32 `json:"crc32"`
			Size           uint64
			Flags          uint16
			Creator        uint16 `json:"creator_version"`
			Reader         uint16 `json:"reader_version"`
			External       uint32 `json:"external_attrs"`
			Internal       uint16 `json:"internal_attrs"`
			Method         uint16
			Extra, Comment []byte
			Date           uint16 `json:"dos_date"`
			Time           uint16 `json:"dos_time"`
			LocalFlags     uint16 `json:"local_flags"`
			LocalExtra     uint16 `json:"local_extra_size"`
		}
	}
	data, err := os.ReadFile("testdata/presentation-zip-metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	p := &opc.Package{}
	for _, c := range fixture.Cases {
		// Inputs deliberately carry metadata that Presentation.save replaces.
		p.Parts = append(p.Parts, &opc.Part{Name: c.Name, Data: c.Data, Header: zip.FileHeader{Method: zip.Store, Comment: "old", Extra: []byte{1, 0, 0, 0}, ExternalAttrs: 0777 << 16}})
	}
	clock := func() time.Time { return time.Date(2024, 2, 3, 4, 5, 7, 0, time.Local) }
	raw, err := presentationFixtureBytes(p, clock)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if len(z.File) != len(fixture.Cases) {
		t.Fatal("member count changed")
	}
	localOffset := int64(0)
	for i, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			f := z.File[i]
			if f.Name != c.Name || f.CRC32 != c.CRC || f.UncompressedSize64 != c.Size || f.Flags != c.Flags || f.CreatorVersion != c.Creator || f.ReaderVersion != c.Reader || f.ExternalAttrs != c.External || f.Method != c.Method || f.ModifiedDate != c.Date || f.ModifiedTime != c.Time || !bytes.Equal(f.Extra, c.Extra) || f.Comment != string(c.Comment) {
				t.Fatalf("metadata mismatch: %+v", f.FileHeader)
			}
			h := raw[localOffset:]
			u16 := binary.LittleEndian.Uint16
			u32 := binary.LittleEndian.Uint32
			if u32(h) != 0x04034b50 || u16(h[6:]) != c.LocalFlags || u16(h[28:]) != c.LocalExtra || u32(h[14:]) != c.CRC || uint64(u32(h[18:])) != f.CompressedSize64 || uint64(u32(h[22:])) != c.Size {
				t.Fatal("local header differs from seekable Python semantics")
			}
			offset, e := f.DataOffset()
			if e != nil {
				t.Fatal(e)
			}
			localOffset = offset + int64(f.CompressedSize64)
			// Next record begins immediately: there is no data descriptor.
			signature := u32(raw[localOffset:])
			if signature != 0x04034b50 && signature != 0x02014b50 {
				t.Fatalf("unexpected trailing record %x", signature)
			}
			rd, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			got, e := io.ReadAll(rd)
			rd.Close()
			if e != nil || !bytes.Equal(got, c.Data) {
				t.Fatal("payload differs", e)
			}
		})
	}
	for _, part := range p.Parts {
		if part.Header.Comment != "old" {
			t.Fatal("input metadata mutated")
		}
	}
	for _, name := range []string{"", "bad\x00name", "directory/"} {
		if _, err := presentationFixtureBytes(&opc.Package{Parts: []*opc.Part{{Name: name}}}, clock); err == nil {
			t.Fatalf("unsupported name %q accepted", name)
		}
	}
}
