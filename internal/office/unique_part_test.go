package office

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestUniquePartNames(t *testing.T) {
	var buf bytes.Buffer
	if err := zip.NewWriter(&buf).Close(); err != nil {
		t.Fatal(err)
	}
	p, err := opc.Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	p.Set("xl/threadedComments/threadedComment1.xml", []byte("<x/>"))
	p.Set("ppt/charts/extendedChart2.xml", []byte("<x/>"))
	for _, c := range []struct{ folder, stem, want string }{
		{"xl/threadedComments", "threadedComment", "xl/threadedComments/threadedComment2.xml"},
		{"xl/persons", "person", "xl/persons/person1.xml"},
		{"ppt", "authors", "ppt/authors1.xml"},
		{"ppt/comments", "modernComment", "ppt/comments/modernComment1.xml"},
		{"ppt/charts", "extendedChart", "ppt/charts/extendedChart1.xml"},
	} {
		if got := uniquePart(p, c.folder, c.stem); got != c.want {
			t.Errorf("%s/%s: got %s want %s", c.folder, c.stem, got, c.want)
		}
	}
}
