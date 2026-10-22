package opc

import (
	"archive/zip"
	"bytes"
	"hash/crc32"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

type member struct {
	name   string
	method uint16
	data   string
	raw    bool // CreateRaw with sizes in the local header (no data descriptor)
}

func buildZIP(t *testing.T, members []member) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range members {
		if m.raw {
			h := &zip.FileHeader{Name: m.name, Method: zip.Store, CRC32: crc32.ChecksumIEEE([]byte(m.data)),
				CompressedSize64: uint64(len(m.data)), UncompressedSize64: uint64(len(m.data))}
			w, err := zw.CreateRaw(h)
			if err != nil {
				t.Fatal(err)
			}
			io.WriteString(w, m.data)
			continue
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: m.name, Method: m.method})
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, m.data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const testCT = `<?xml version="1.0"?><Types xmlns="` + NSCT + `"><Default Extension="xml" ContentType="application/xml"/></Types>`

// members returns the written ZIP central directory keyed by name.
func members(t *testing.T, data []byte) map[string]*zip.File {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*zip.File{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, rc); err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		rc.Close()
		out[f.Name] = f
	}
	return out
}

func rawBytes(t *testing.T, f *zip.File) []byte {
	t.Helper()
	r, err := f.OpenRaw()
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBytesRawCopiesCleanDeflateMembers(t *testing.T) {
	src := buildZIP(t, []member{
		{name: contentTypesName, method: zip.Deflate, data: testCT},
		{name: "media/a.bin", method: zip.Deflate, data: strings.Repeat("abc", 1000)},
		{name: "media/stored.bin", raw: true, data: "stored-with-sizes"},
	})
	p, err := Read(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	in, got := members(t, src), members(t, out)
	for _, name := range []string{"media/a.bin", "media/stored.bin"} {
		if !bytes.Equal(rawBytes(t, in[name]), rawBytes(t, got[name])) || in[name].Method != got[name].Method {
			t.Fatalf("%s was not raw-copied", name)
		}
		if got[name].Flags&0x8 != 0 {
			t.Fatalf("%s copied with data descriptor", name)
		}
	}
}

func TestBytesStoredDataDescriptorReencoded(t *testing.T) {
	src := buildZIP(t, []member{
		{name: contentTypesName, method: zip.Deflate, data: testCT},
		{name: "_rels/.rels", method: zip.Store, data: `<Relationships xmlns="` + NSRel + `"/>`},
	})
	if f := members(t, src)["_rels/.rels"]; f.Method != zip.Store || f.Flags&0x8 == 0 {
		t.Fatal("fixture must be stored with a data descriptor")
	}
	p, err := Read(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	f := members(t, out)["_rels/.rels"]
	if f.Method != zip.Deflate {
		t.Fatalf("_rels/.rels method %d, want deflate", f.Method)
	}
	q, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(q.Get("_rels/.rels"), p.Get("_rels/.rels")) {
		t.Fatal("_rels/.rels changed")
	}
}

func TestBytesRecompressesModifiedParts(t *testing.T) {
	src := buildZIP(t, []member{
		{name: contentTypesName, method: zip.Deflate, data: testCT},
		{name: "a.xml", method: zip.Deflate, data: "<a/>"},
		{name: "b.xml", method: zip.Deflate, data: "<b/>"},
		{name: "c.xml", method: zip.Deflate, data: "<c/>"},
		{name: "same-len.bin", method: zip.Deflate, data: "AAAA"},
		{name: "in-place.bin", method: zip.Deflate, data: "BBBB"},
		{name: "stored.bin", raw: true, data: "CCCC"},
	})
	p, err := Read(src)
	if err != nil {
		t.Fatal(err)
	}
	p.Set("a.xml", []byte("<a changed='1'/>"))
	doc := etree.NewDocument()
	doc.CreateElement("b").CreateAttr("x", "y")
	if err := p.SetXML("b.xml", doc); err != nil {
		t.Fatal(err)
	}
	p.Remove("c.xml")
	p.Set("new.xml", []byte("<new/>"))
	if err := p.EnsureContentType("new.xml", "application/x-new+xml"); err != nil {
		t.Fatal(err)
	}
	p.index["same-len.bin"].Data = []byte("ZZZZ")
	p.index["in-place.bin"].Data[0] = 'Q'
	p.index["stored.bin"].Data = []byte("DDDD")

	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	got := members(t, out) // Open+full read verifies CRC/sizes
	if _, ok := got["c.xml"]; ok {
		t.Fatal("removed part written")
	}
	q, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Parts) != len(p.Parts) {
		t.Fatalf("got %d parts, want %d", len(q.Parts), len(p.Parts))
	}
	for _, part := range p.Parts {
		if !bytes.Equal(q.Get(part.Name), part.Data) {
			t.Fatalf("%s: got %q want %q", part.Name, q.Get(part.Name), part.Data)
		}
		f := got[part.Name]
		if f.CRC32 != crc32.ChecksumIEEE(part.Data) || f.UncompressedSize64 != uint64(len(part.Data)) {
			t.Fatalf("%s: wrong CRC/size", part.Name)
		}
	}
	if !strings.Contains(string(q.Get(contentTypesName)), "application/x-new+xml") {
		t.Fatal("content types not rewritten")
	}
	for _, name := range []string{"a.xml", "b.xml", "new.xml", "same-len.bin", "in-place.bin", "stored.bin", contentTypesName} {
		if got[name].Method != zip.Deflate {
			t.Fatalf("%s not freshly deflated", name)
		}
	}
}

func TestReadDoesNotAliasCallerBuffer(t *testing.T) {
	src := buildZIP(t, []member{{name: "a.bin", raw: true, data: "original"}})
	p, err := Read(src)
	if err != nil {
		t.Fatal(err)
	}
	for i := range src {
		src[i] = 0
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	q, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(q.Get("a.bin")) != "original" {
		t.Fatalf("got %q", q.Get("a.bin"))
	}
}

func TestBytesPreservesNonASCIINames(t *testing.T) {
	names := []string{"word/media/图片.png", "ppt/média/ünï.xml"}
	src := buildZIP(t, []member{
		{name: names[0], method: zip.Deflate, data: "one"},
		{name: names[1], method: zip.Deflate, data: "two"},
	})
	p, err := Read(src)
	if err != nil {
		t.Fatal(err)
	}
	p.Set(names[1], []byte("changed"))
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	got := members(t, out)
	for _, name := range names {
		f := got[name]
		if f == nil || f.Flags&0x800 == 0 {
			t.Fatalf("%s missing or without UTF-8 flag", name)
		}
	}
	q, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(q.Get(names[0])) != "one" || string(q.Get(names[1])) != "changed" {
		t.Fatal("non-ASCII part data changed")
	}
}

func TestBytesRoundTripFixtures(t *testing.T) {
	var files []string
	filepath.WalkDir("../../testdata", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			switch strings.ToLower(filepath.Ext(path)) {
			case ".docx", ".xlsx", ".pptx":
				files = append(files, path)
			}
		}
		return nil
	})
	if len(files) == 0 {
		t.Fatal("no fixtures found")
	}
	checked := 0
	for _, file := range files {
		p, err := Open(file)
		if err != nil {
			continue // deliberately malformed fixtures
		}
		out, err := p.Bytes()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		q, err := Read(out)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if len(q.Parts) != len(p.Parts) {
			t.Fatalf("%s: part count changed", file)
		}
		for i, part := range p.Parts {
			if q.Parts[i].Name != part.Name || !bytes.Equal(q.Parts[i].Data, part.Data) {
				t.Fatalf("%s: part %s changed", file, part.Name)
			}
			if q.Parts[i].Header.Method == zip.Store && q.Parts[i].Header.Flags&0x8 != 0 {
				t.Fatalf("%s: stored streaming member %s", file, part.Name)
			}
		}
		checked++
	}
	t.Logf("round-tripped %d fixtures", checked)
}
