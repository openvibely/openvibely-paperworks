package opc

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenBytesAndSaveNewPreservePartsAndRawMembers(t *testing.T) {
	media := strings.Repeat("large-media-payload-", (4<<20)/len("large-media-payload-"))
	source := buildZIP(t, []member{
		{name: contentTypesName, method: zip.Deflate, data: testCT},
		{name: "word/document.xml", method: zip.Deflate, data: "<document>before</document>"},
		{name: "word/media/image1.bin", method: zip.Deflate, data: media},
	})
	input := filepath.Join(t.TempDir(), "source.docx")
	if err := os.WriteFile(input, source, 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := Open(input)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{contentTypesName, "word/document.xml", "word/media/image1.bin"}
	if got := p.Names(); !equalStrings(got, wantNames) {
		t.Fatalf("part names = %v, want %v", got, wantNames)
	}
	if got := string(p.Get("word/media/image1.bin")); got != media {
		t.Fatal("expanded media bytes changed after Open")
	}

	bytesOutput, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	bytesPackage, err := Read(bytesOutput)
	if err != nil {
		t.Fatal(err)
	}
	if got := bytesPackage.Names(); !equalStrings(got, wantNames) {
		t.Fatalf("Bytes part names = %v, want %v", got, wantNames)
	}
	if !bytes.Equal(bytesPackage.Get("word/media/image1.bin"), []byte(media)) {
		t.Fatal("Bytes changed expanded media")
	}
	assertRawMemberEqual(t, source, bytesOutput, "word/media/image1.bin")

	savedPath := filepath.Join(t.TempDir(), "saved.docx")
	if err := p.SaveNew(savedPath); err != nil {
		t.Fatal(err)
	}
	savedOutput, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	savedPackage, err := Read(savedOutput)
	if err != nil {
		t.Fatal(err)
	}
	if got := savedPackage.Names(); !equalStrings(got, wantNames) {
		t.Fatalf("SaveNew part names = %v, want %v", got, wantNames)
	}
	if !bytes.Equal(savedPackage.Get("word/media/image1.bin"), []byte(media)) {
		t.Fatal("SaveNew changed expanded media")
	}
	assertRawMemberEqual(t, source, savedOutput, "word/media/image1.bin")
}

func TestOpenRejectsMemberCRCFailure(t *testing.T) {
	archive := buildZIP(t, []member{{name: "media/a.bin", method: zip.Deflate, data: "original"}})
	corruptCentralCRC(t, archive, "media/a.bin")
	file := filepath.Join(t.TempDir(), "bad-crc.docx")
	if err := os.WriteFile(file, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(file); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("Open error = %v, want checksum error", err)
	}
}

func TestOpenRejectsMalformedZIP(t *testing.T) {
	file := filepath.Join(t.TempDir(), "malformed.docx")
	if err := os.WriteFile(file, []byte("not a zip archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(file); err == nil || !strings.Contains(err.Error(), "not a valid Office package (zip)") {
		t.Fatalf("Open error = %v, want malformed ZIP error", err)
	}
}

func TestOpenEnforcesAdvertisedSizeLimits(t *testing.T) {
	tests := []struct {
		name  string
		parts []member
		sizes map[string]uint32
		want  string
	}{
		{
			name:  "part size",
			parts: []member{{name: "large.bin", method: zip.Deflate, data: "x"}},
			sizes: map[string]uint32{"large.bin": MaxPartBytes + 1},
			want:  "part too large",
		},
		{
			name: "total size",
			parts: []member{
				{name: "one.bin", method: zip.Deflate, data: "x"},
				{name: "two.bin", method: zip.Deflate, data: "x"},
				{name: "three.bin", method: zip.Deflate, data: "x"},
			},
			sizes: map[string]uint32{
				"one.bin":   MaxPartBytes,
				"two.bin":   MaxPartBytes,
				"three.bin": 1,
			},
			want: "inspection limits",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildZIP(t, tc.parts)
			setAdvertisedSizes(t, archive, tc.sizes)
			file := filepath.Join(t.TempDir(), "oversized.docx")
			if err := os.WriteFile(file, archive, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(file); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Open error = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func corruptCentralCRC(t *testing.T, archive []byte, name string) {
	t.Helper()
	const centralHeaderSig = "PK\x01\x02"
	for offset := 0; ; {
		rel := bytes.Index(archive[offset:], []byte(centralHeaderSig))
		if rel < 0 {
			break
		}
		offset += rel
		if offset+46 > len(archive) {
			t.Fatalf("truncated central directory header at %d", offset)
		}
		nameLen := int(binary.LittleEndian.Uint16(archive[offset+28 : offset+30]))
		extraLen := int(binary.LittleEndian.Uint16(archive[offset+30 : offset+32]))
		commentLen := int(binary.LittleEndian.Uint16(archive[offset+32 : offset+34]))
		end := offset + 46 + nameLen + extraLen + commentLen
		if end > len(archive) {
			t.Fatalf("truncated central directory entry at %d", offset)
		}
		if string(archive[offset+46:offset+46+nameLen]) == name {
			archive[offset+16] ^= 0xff
			return
		}
		offset = end
	}
	t.Fatalf("central directory entry %q not found", name)
}

func setAdvertisedSizes(t *testing.T, archive []byte, sizes map[string]uint32) {
	t.Helper()
	const centralHeaderSig = "PK\x01\x02"
	found := make(map[string]bool, len(sizes))
	for offset := 0; ; {
		rel := bytes.Index(archive[offset:], []byte(centralHeaderSig))
		if rel < 0 {
			break
		}
		offset += rel
		if offset+46 > len(archive) {
			t.Fatalf("truncated central directory header at %d", offset)
		}
		nameLen := int(binary.LittleEndian.Uint16(archive[offset+28 : offset+30]))
		extraLen := int(binary.LittleEndian.Uint16(archive[offset+30 : offset+32]))
		commentLen := int(binary.LittleEndian.Uint16(archive[offset+32 : offset+34]))
		end := offset + 46 + nameLen + extraLen + commentLen
		if end > len(archive) {
			t.Fatalf("truncated central directory entry at %d", offset)
		}
		name := string(archive[offset+46 : offset+46+nameLen])
		if size, ok := sizes[name]; ok {
			binary.LittleEndian.PutUint32(archive[offset+24:offset+28], size)
			found[name] = true
		}
		offset = end
	}
	for name := range sizes {
		if !found[name] {
			t.Fatalf("central directory entry %q not found", name)
		}
	}
}

func assertRawMemberEqual(t *testing.T, before, after []byte, name string) {
	t.Helper()
	beforeMembers, afterMembers := members(t, before), members(t, after)
	beforeRaw, afterRaw := rawBytes(t, beforeMembers[name]), rawBytes(t, afterMembers[name])
	if !bytes.Equal(beforeRaw, afterRaw) {
		t.Fatalf("compressed member %s changed", name)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
