package wordedit

import (
	"bytes"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"os"
	"reflect"
	"testing"
)

func TestReferenceSplitFieldHelper(t *testing.T) {
	cases := []struct {
		name     string
		code     []string
		complete bool
		want     string
	}{
		{"0-False", []string{" RE", "F A "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF A\", \"incomplete\": true}]"},
		{"0-True", []string{" RE", "F A "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF A\"}]"},
		{"1-False", []string{" IF A", " < B "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"IF A < B\", \"incomplete\": true}]"},
		{"1-True", []string{" IF A", " < B "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"IF A < B\"}]"},
		{"2-False", []string{" A ", "& B "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"A & B\", \"incomplete\": true}]"},
		{"2-True", []string{" A ", "& B "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"A & B\"}]"},
		{"3-False", []string{"\"quo", "ted\""}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"\\\"quoted\\\"\", \"incomplete\": true}]"},
		{"3-True", []string{"\"quo", "ted\""}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"\\\"quoted\\\"\"}]"},
		{"4-False", []string{"'sin", "gle'"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"'single'\", \"incomplete\": true}]"},
		{"4-True", []string{"'sin", "gle'"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"'single'\"}]"},
		{"5-False", []string{" Ca", "fé "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"Café\", \"incomplete\": true}]"},
		{"5-True", []string{" Ca", "fé "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"Café\"}]"},
		{"6-False", []string{" 中", "文 "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"中文\", \"incomplete\": true}]"},
		{"6-True", []string{" 中", "文 "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"中文\"}]"},
		{"7-False", []string{" ", "😀 "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"😀\", \"incomplete\": true}]"},
		{"7-True", []string{" ", "😀 "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"😀\"}]"},
		{"8-False", []string{"\tRE", "F\tA\n"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF A\", \"incomplete\": true}]"},
		{"8-True", []string{"\tRE", "F\tA\n"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF A\"}]"},
		{"9-False", []string{"", ""}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"\", \"incomplete\": true}]"},
		{"9-True", []string{"", ""}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"\"}]"},
		{"10-False", []string{" PA", "GE "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"PAGE\", \"incomplete\": true}]"},
		{"10-True", []string{" PA", "GE "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"PAGE\"}]"},
		{"11-False", []string{" NUMP", "AGES "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"NUMPAGES\", \"incomplete\": true}]"},
		{"11-True", []string{" NUMP", "AGES "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"NUMPAGES\"}]"},
		{"12-False", []string{" SEQ F", "igure "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"SEQ Figure\", \"incomplete\": true}]"},
		{"12-True", []string{" SEQ F", "igure "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"SEQ Figure\"}]"},
		{"13-False", []string{" REF", " a>b "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF a>b\", \"incomplete\": true}]"},
		{"13-True", []string{" REF", " a>b "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF a>b\"}]"},
		{"14-False", []string{" REF", " a=b "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF a=b\", \"incomplete\": true}]"},
		{"14-True", []string{" REF", " a=b "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF a=b\"}]"},
		{"15-False", []string{" REF", " [x] "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF [x]\", \"incomplete\": true}]"},
		{"15-True", []string{" REF", " [x] "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"REF [x]\"}]"},
		{"16-False", []string{" a ", " b "}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"a b\", \"incomplete\": true}]"},
		{"16-True", []string{" a ", " b "}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"a b\"}]"},
		{"17-False", []string{"", "0"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"0\", \"incomplete\": true}]"},
		{"17-True", []string{"", "0"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"0\"}]"},
		{"18-False", []string{"-", "1"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"-1\", \"incomplete\": true}]"},
		{"18-True", []string{"-", "1"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"-1\"}]"},
		{"19-False", []string{"x", "/y"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"x/y\", \"incomplete\": true}]"},
		{"19-True", []string{"x", "/y"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"x/y\"}]"},
		{"20-False", []string{"x", "\\y"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"x\\\\y\", \"incomplete\": true}]"},
		{"20-True", []string{"x", "\\y"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"x\\\\y\"}]"},
		{"21-False", []string{"x", ":y"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"x:y\", \"incomplete\": true}]"},
		{"21-True", []string{"x", ":y"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"x:y\"}]"},
		{"22-False", []string{"x", ".y"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"x.y\", \"incomplete\": true}]"},
		{"22-True", []string{"x", ".y"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"x.y\"}]"},
		{"23-False", []string{"x_", "{y}"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"x_{y}\", \"incomplete\": true}]"},
		{"23-True", []string{"x_", "{y}"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"x_{y}\"}]"},
		{"24-False", []string{"e", "nd"}, false, "[{\"part\": \"word/document.xml\", \"instruction\": \"end\", \"incomplete\": true}]"},
		{"24-True", []string{"e", "nd"}, true, "[{\"part\": \"word/document.xml\", \"instruction\": \"end\"}]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := mk(t, "split-helper", "<w:p>"+referenceSplitField(c.code, c.complete)+"</w:p>", nil)
			before, e := os.ReadFile(src)
			if e != nil {
				t.Fatal(e)
			}
			got, e := office.PackageReport(src)
			if e != nil {
				t.Fatal(e)
			}
			var want, actual any
			if e = json.Unmarshal([]byte(c.want), &want); e != nil {
				t.Fatal(e)
			}
			raw, e := json.Marshal(got["fields"])
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(raw, &actual); e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("fields=%s want=%s", raw, c.want)
			}
			after, e := os.ReadFile(src)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", e)
			}
		})
	}
}
