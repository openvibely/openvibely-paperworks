package office

import (
	"github.com/beevik/etree"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func referenceWorksheetState(t *testing.T, filename, name string) string {
	t.Helper()
	z := referencePreviewZIP(t, filename)
	main := referencePackageTarget(t, z, "", "", NSR+"/officeDocument")
	for _, sheet := range Children(Child(referencePackageXML(t, z, main), NSS, "sheets"), NSS, "sheet") {
		if sheet.SelectAttrValue("name", "") == name {
			return sheet.SelectAttrValue("state", "visible")
		}
	}
	t.Fatal("worksheet absent", name)
	return ""
}

// openpyxl 3.1.5 HeaderFooterItem.from_tree/_split_string distinguishes absent
// center (None) from empty parsed text (including formatting-only and bad input).
// Only the None predicate is needed; this does not implement header rendering.
func referenceOddHeaderCenterIsNone(t *testing.T, ws *etree.Element) bool {
	t.Helper()
	odd := Child(Child(ws, NSS, "headerFooter"), NSS, "oddHeader")
	if odd == nil || odd.Text() == "" {
		return true
	}
	text := regexp.MustCompile(`_x([0-9A-Fa-f]{4})_`).ReplaceAllStringFunc(odd.Text(), func(s string) string {
		n, _ := strconv.ParseInt(s[2:6], 16, 32)
		if n >= 0xd800 && n <= 0xdfff {
			t.Fatal("unpaired header surrogate outside reviewed reader scope")
		}
		return string(rune(n))
	})
	// Reference's $ also matches before a final newline. All groups are DOTALL.
	pattern := regexp.MustCompile(`(?s)^(?:&L(.+?))?(?:&C(.+?))?(?:&R(.+?))?$`)
	match := pattern.FindStringSubmatchIndex(text)
	if match == nil && strings.HasSuffix(text, "\n") {
		match = pattern.FindStringSubmatchIndex(strings.TrimSuffix(text, "\n"))
	}
	return match != nil && match[4] == -1
}

func TestReferenceWorksheetHeaderNoneContract(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want bool
	}{
		{"", true},
		{"&Lleft", true},
		{"&Rright", true},
		{"&Ccenter", false},
		{"&Lleft&Ccenter&Rright", false},
		{"&C", false},
		{"broken", false},
		{"&C&12", false},
		{"&C&\"Arial\"", false},
		{"&Lleft_x000a_", true},
		{"_x000a_", true},
		{"_x0026_Cencoded", false},
		{"&Lleft&Rright", true},
		{"&Lleft&C", true},
		{"&C&K000000", false},
	} {
		t.Run(c.raw, func(t *testing.T) {
			ws := etree.NewElement("worksheet")
			ws.CreateAttr("xmlns", NSS)
			ws.CreateElement("headerFooter").CreateElement("oddHeader").SetText(c.raw)
			if got := referenceOddHeaderCenterIsNone(t, ws); got != c.want {
				t.Fatalf("None=%v want %v", got, c.want)
			}
		})
	}
}
