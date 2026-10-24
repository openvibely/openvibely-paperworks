package cli

import (
	"bytes"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"reflect"
	"testing"
)

func TestPDFFontsCommand(t *testing.T) {
	var out, errs bytes.Buffer
	if code := Main([]string{"pdf-fonts", "../../testdata/reference-mirror/runtime-font-render.pdf"}, &out, &errs); code != 0 {
		t.Fatalf("%d: %s", code, errs.String())
	}
	var got []pdf.PDFFont
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Helvetica-Bold" || got[0].Embedded || !reflect.DeepEqual(got[0].Pages, []int{1}) {
		t.Fatalf("%+v", got)
	}
	out.Reset()
	errs.Reset()
	if Main([]string{"pdf-fonts"}, &out, &errs) == 0 {
		t.Fatal("accepted missing input")
	}
}
