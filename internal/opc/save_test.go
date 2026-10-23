package opc

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveNewPreservesPartsWithIndependentZIPReader(t *testing.T) {
	source := buildZIP(t, []member{
		{name: "[Content_Types].xml", method: zip.Deflate, data: testCT},
		{name: "word/document.xml", method: zip.Deflate, data: "<document>before</document>"},
		{name: "word/media/image1.bin", method: zip.Deflate, data: strings.Repeat("media-one", 1024)},
		{name: "word/media/image2.bin", method: zip.Deflate, data: strings.Repeat("media-two", 2048)},
	})
	pkg, err := Read(source)
	if err != nil {
		t.Fatal(err)
	}
	pkg.Set("word/document.xml", []byte("<document>after</document>"))

	wantNames := make([]string, len(pkg.Parts))
	wantParts := make(map[string][]byte, len(pkg.Parts))
	for i, part := range pkg.Parts {
		wantNames[i] = part.Name
		wantParts[part.Name] = append([]byte(nil), part.Data...)
	}

	output := filepath.Join(t.TempDir(), "saved.docx")
	if err := pkg.SaveNew(output); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, 0, len(zr.File))
	gotParts := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		if _, exists := gotParts[f.Name]; exists {
			t.Fatalf("duplicate saved part %q", f.Name)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", f.Name, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", f.Name, closeErr)
		}
		gotNames = append(gotNames, f.Name)
		gotParts[f.Name] = data
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("saved part names = %v, want %v", gotNames, wantNames)
	}
	if !reflect.DeepEqual(gotParts, wantParts) {
		t.Fatalf("saved uncompressed part bytes differ")
	}
}

func TestSaveNewRejectsUnsafeAndDuplicateModifiedParts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Package)
		want   string
	}{
		{
			name: "unsafe name",
			mutate: func(p *Package) {
				p.Set("word/../escape.xml", []byte("bad"))
			},
			want: "unsafe part name",
		},
		{
			name: "duplicate name",
			mutate: func(p *Package) {
				p.Parts = append(p.Parts, &Part{Name: p.Parts[0].Name, Data: []byte("duplicate")})
			},
			want: "duplicate part",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Read(buildZIP(t, []member{{name: "original.xml", method: zip.Deflate, data: "<original/>"}}))
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(p)
			output := filepath.Join(t.TempDir(), "rejected.zip")
			if err := p.SaveNew(output); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("SaveNew error = %v, want error containing %q", err, tc.want)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("invalid package was written: %v", err)
			}
		})
	}
}

func TestValidatePackageMembersRetainsSafetyLimits(t *testing.T) {
	tests := []struct {
		name    string
		members []packageMember
		want    string
	}{
		{
			name: "part count",
			members: func() []packageMember {
				members := make([]packageMember, MaxParts+1)
				for i := range members {
					members[i] = packageMember{name: "part-" + strings.Repeat("x", i%3), size: 0}
					members[i].name += string(rune('a'+i%26)) + string(rune('A'+i/26))
				}
				return members
			}(),
			want: "inspection limits",
		},
		{
			name: "total size",
			members: []packageMember{
				{name: "one.bin", size: MaxPartBytes},
				{name: "two.bin", size: MaxPartBytes},
				{name: "three.bin", size: 1},
			},
			want: "inspection limits",
		},
		{
			name:    "part size",
			members: []packageMember{{name: "large.bin", size: MaxPartBytes + 1}},
			want:    "part too large",
		},
		{
			name:    "unsafe name",
			members: []packageMember{{name: "../unsafe", size: 0}},
			want:    "unsafe part name",
		},
		{
			name:    "duplicate name",
			members: []packageMember{{name: "same.xml"}, {name: "same.xml"}},
			want:    "duplicate part",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePackageMembers(tc.members)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validation error = %v, want error containing %q", err, tc.want)
			}
		})
	}
}
