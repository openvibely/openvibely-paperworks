package wordedit

import (
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"reflect"
	"testing"
)

func TestReferenceComplexFieldAcquisition(t *testing.T) {
	cases := []struct {
		name, instruction, cached, want string
		fragments                       []string
	}{
		{"0-False", " REF A ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"REF A\"}]", []string{" RE", "F A "}},
		{"0-True", "😀 REF A ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 REF A\"}]", []string{"😀 R", "EF A "}},
		{"1-False", " IF A < B ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"IF A < B\"}]", []string{" IF", " A < B "}},
		{"1-True", "😀 IF A < B ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 IF A < B\"}]", []string{"😀 I", "F A < B "}},
		{"2-False", " A & B ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"A & B\"}]", []string{" A ", "& B "}},
		{"2-True", "😀 A & B ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 A & B\"}]", []string{"😀 A", " & B "}},
		{"3-False", "\"quoted\"", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\\"quoted\\\"\"}]", []string{"\"qu", "oted\""}},
		{"3-True", "😀\"quoted\"", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00\\\"quoted\\\"\"}]", []string{"😀\"q", "uoted\""}},
		{"4-False", "'single'", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"'single'\"}]", []string{"'si", "ngle'"}},
		{"4-True", "😀'single'", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00'single'\"}]", []string{"😀's", "ingle'"}},
		{"5-False", " Caf\u00e9 ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"Caf\\u00e9\"}]", []string{" Ca", "f\u00e9 "}},
		{"5-True", "😀 Caf\u00e9 ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 Caf\\u00e9\"}]", []string{"😀 C", "af\u00e9 "}},
		{"6-False", " \u4e2d\u6587 ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\u4e2d\\u6587\"}]", []string{" \u4e2d\u6587", " "}},
		{"6-True", "😀 \u4e2d\u6587 ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 \\u4e2d\\u6587\"}]", []string{"😀 \u4e2d", "\u6587 "}},
		{"7-False", " 😀 ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00\"}]", []string{" 😀 ", ""}},
		{"7-True", "😀 😀 ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 \\ud83d\\ude00\"}]", []string{"😀 😀", " "}},
		{"8-False", "\tREF\tA\n", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"REF A\"}]", []string{"\tRE", "F\tA\n"}},
		{"8-True", "😀\tREF\tA\n", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 REF A\"}]", []string{"😀\tR", "EF\tA\n"}},
		{"9-False", "", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"\"}]", []string{"", ""}},
		{"9-True", "😀", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00\"}]", []string{"😀", ""}},
		{"10-False", " PAGE ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"PAGE\"}]", []string{" PA", "GE "}},
		{"10-True", "😀 PAGE ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 PAGE\"}]", []string{"😀 P", "AGE "}},
		{"11-False", " NUMPAGES ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"NUMPAGES\"}]", []string{" NU", "MPAGES "}},
		{"11-True", "😀 NUMPAGES ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 NUMPAGES\"}]", []string{"😀 N", "UMPAGES "}},
		{"12-False", " SEQ Figure ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"SEQ Figure\"}]", []string{" SE", "Q Figure "}},
		{"12-True", "😀 SEQ Figure ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 SEQ Figure\"}]", []string{"😀 S", "EQ Figure "}},
		{"13-False", " REF a>b ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"REF a>b\"}]", []string{" RE", "F a>b "}},
		{"13-True", "😀 REF a>b ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 REF a>b\"}]", []string{"😀 R", "EF a>b "}},
		{"14-False", " REF a=b ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"REF a=b\"}]", []string{" RE", "F a=b "}},
		{"14-True", "😀 REF a=b ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 REF a=b\"}]", []string{"😀 R", "EF a=b "}},
		{"15-False", " REF [x] ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"REF [x]\"}]", []string{" RE", "F [x] "}},
		{"15-True", "😀 REF [x] ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 REF [x]\"}]", []string{"😀 R", "EF [x] "}},
		{"16-False", " a  b ", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"a b\"}]", []string{" a ", " b "}},
		{"16-True", "😀 a  b ", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00 a b\"}]", []string{"😀 a", "  b "}},
		{"17-False", "0", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"0\"}]", []string{"0", ""}},
		{"17-True", "😀0", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude000\"}]", []string{"😀0", ""}},
		{"18-False", "-1", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"-1\"}]", []string{"-1", ""}},
		{"18-True", "😀-1", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00-1\"}]", []string{"😀-1", ""}},
		{"19-False", "x/y", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"x/y\"}]", []string{"x/y", ""}},
		{"19-True", "😀x/y", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00x/y\"}]", []string{"😀x/", "y"}},
		{"20-False", "x\\y", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"x\\\\y\"}]", []string{"x\\y", ""}},
		{"20-True", "😀x\\y", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00x\\\\y\"}]", []string{"😀x\\", "y"}},
		{"21-False", "x:y", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"x:y\"}]", []string{"x:y", ""}},
		{"21-True", "😀x:y", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00x:y\"}]", []string{"😀x:", "y"}},
		{"22-False", "x.y", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"x.y\"}]", []string{"x.y", ""}},
		{"22-True", "😀x.y", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00x.y\"}]", []string{"😀x.", "y"}},
		{"23-False", "x_{y}", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"x_{y}\"}]", []string{"x_{", "y}"}},
		{"23-True", "😀x_{y}", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00x_{y}\"}]", []string{"😀x_", "{y}"}},
		{"24-False", "end", "0", "[{\"part\": \"word/document.xml\", \"instruction\": \"end\"}]", []string{"end", ""}},
		{"24-True", "😀end", "A & <B>", "[{\"part\": \"word/document.xml\", \"instruction\": \"\\ud83d\\ude00end\"}]", []string{"😀en", "d"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := mk(t, "complex", "<w:p>"+complexField(c.instruction, c.cached)+"</w:p>", nil)
			root := referenceXML(t, src, "word/document.xml")
			var fragments []string
			for _, e := range office.All(root, nsW, "instrText") {
				fragments = append(fragments, e.Text())
			}
			if !reflect.DeepEqual(fragments, c.fragments) {
				t.Fatalf("fragments %q want %q", fragments, c.fragments)
			}
			report, e := office.PackageReport(src)
			if e != nil {
				t.Fatal(e)
			}
			raw, e := json.Marshal(report["fields"])
			if e != nil {
				t.Fatal(e)
			}
			var got, want any
			json.Unmarshal(raw, &got)
			json.Unmarshal([]byte(c.want), &want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s != %s", raw, c.want)
			}
		})
	}
}
