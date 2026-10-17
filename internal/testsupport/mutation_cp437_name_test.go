package testsupport

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Legacy names must reach the callback as decoded Reference dictionary keys.
func TestMutationCP437Name(t *testing.T) {
	raw, err := StoredZIPBytes(&OrderedZIP{Parts: []ZIPPart{{"original", "keep"}}})
	if err != nil {
		t.Fatal(err)
	}
	central := bytes.Index(raw, []byte("PK\x01\x02"))
	if central < 0 {
		t.Fatal("central header missing")
	}
	binary.LittleEndian.PutUint16(raw[6:8], 0)
	binary.LittleEndian.PutUint16(raw[central+8:central+10], 0)
	raw[30], raw[central+46] = 130, 130
	name := "ériginal"
	root := t.TempDir()
	src, dst := filepath.Join(root, "source.zip"), filepath.Join(root, "2.pptx")
	if err := os.WriteFile(src, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	mutate := func() error {
		z, err := zip.OpenReader(src)
		if err != nil {
			return err
		}
		defer z.Close()
		parts, err := ReadOrderedZIP(z)
		if err != nil {
			return err
		}
		callbacks++
		if len(parts.Parts) != 1 || parts.Parts[0] != (ZIPPart{name, "keep"}) {
			t.Fatalf("callback parts: %#v", parts.Parts)
		}
		parts.Set("added", []byte("value"))
		return SaveStoredZIP(dst, parts)
	}
	if err := mutate(); err != nil {
		t.Fatal(err)
	}
	if callbacks != 1 {
		t.Fatalf("callbacks=%d", callbacks)
	}
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, raw) {
		t.Fatal("source changed")
	}
	after, err = os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(after, []byte("sentinel")) {
		t.Fatal("expected documented destination overwrite")
	}
	z, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	parts, err := ReadOrderedZIP(z)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts.Parts) != 2 || parts.Parts[0] != (ZIPPart{name, "keep"}) || parts.Parts[1] != (ZIPPart{"added", "value"}) {
		t.Fatalf("saved parts: %#v", parts.Parts)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "2.pptx" || entries[1].Name() != "source.zip" {
		t.Fatal("directory entries changed")
	}
	if z.File[0].Flags&0x800 == 0 {
		t.Fatal("decoded saved name missing UTF-8 flag")
	}
	t.Logf("saved_zip_base64=%s", base64.StdEncoding.EncodeToString(after))
	// Hex avoids replacement-character normalization when the JSON test log
	// encodes Go's invalid UTF-8 string. Record the actual saved UTF-8 flag too.
	t.Logf("callback_count=%d callback_name_hex=%x callback_payload_hex=%x error=nil source_unchanged=true destination_unchanged=false entries_unchanged=true", callbacks, []byte(name), []byte("keep"))
	for i, member := range z.File {
		t.Logf("saved_member=%d name_hex=%x flags=%04x", i, []byte(member.Name), member.Flags)
	}
	t.Log("CP437 callback and decoded saved names match Python; container bytes and unrelated acquisition gaps remain open")
}

func TestMutationCP437DecodedDuplicate(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, tc := range []struct {
		name, data string
		legacy     bool
	}{
		{string([]byte{0x82}), "shadowed", true}, {"other", "middle", false}, {"é", "last", false},
	} {
		f, err := w.CreateHeader(&zip.FileHeader{Name: tc.name, Method: zip.Store, NonUTF8: tc.legacy})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(tc.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "duplicate.zip")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	parts, err := ReadOrderedZIP(z)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts.Parts) != 2 || parts.Parts[0] != (ZIPPart{"é", "last"}) || parts.Parts[1] != (ZIPPart{"other", "middle"}) {
		t.Fatalf("decoded dictionary: %#v", parts.Parts)
	}
	t.Logf("input_zip_base64=%s", base64.StdEncoding.EncodeToString(b.Bytes()))
}
