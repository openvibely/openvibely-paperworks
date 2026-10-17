package docx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestHeaderWeightIndependentOfRepeat(t *testing.T) {
	no, yes := false, true
	for _, tc := range []struct {
		name                 string
		repeat, bold         *bool
		wantRepeat, wantBold bool
	}{
		{"default", nil, nil, true, true},
		{"plain-repeat", nil, &no, true, false},
		{"explicit-bold", &yes, &yes, true, true},
		{"not-header", &no, &yes, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Build(Spec{Blocks: []Block{{Type: "table", Rows: [][]string{{"Header"}, {"Body"}}, Header: tc.repeat, HeaderBold: tc.bold}}}, "")
			if err != nil {
				t.Fatal(err)
			}
			z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range z.File {
				if f.Name == "word/document.xml" {
					r, err := f.Open()
					if err != nil {
						t.Fatal(err)
					}
					b, err := io.ReadAll(r)
					r.Close()
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(b), "<w:tblHeader/>") != tc.wantRepeat {
						t.Fatal("repeating-header property changed")
					}
					want := 0
					if tc.wantBold {
						want = 1
					}
					if got := strings.Count(string(b), "<w:b/>"); got != want {
						t.Fatalf("bold runs %d, want %d", got, want)
					}
					return
				}
			}
			t.Fatal("document missing")
		})
	}
}
