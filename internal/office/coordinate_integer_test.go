package office

import (
	"github.com/beevik/etree"
	"testing"
)

func TestCoordinateIntegerLexicalContract(t *testing.T) {
	for _, s := range []string{"1002", "+1002", " 1002 ", "01002", "1_002", "١٠٠٢", "１００２", "\u00a01002\u00a0"} {
		v, e := coordinateInteger(s)
		if e != nil || v != 1002 {
			t.Errorf("%q: %v %v", s, v, e)
		}
	}
	for _, s := range []string{"1002.0", "1002e0", "", "NaN", "1__002", "_1002", "1002_", "+ 1002", "\x1c1002", "²"} {
		if _, e := coordinateInteger(s); e == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestCoordinateIntegerAcquisition(t *testing.T) {
	for _, tc := range []struct {
		value            string
		reject, overflow bool
	}{{"1002.0", true, false}, {" 1002 ", false, true}, {"١٠٠٢", false, true}, {"0", false, false}} {
		d := etree.NewDocument()
		if e := d.ReadFromString(`<p:sld xmlns:p="` + NSP + `" xmlns:a="` + NSA + `"><p:sp><p:spPr><a:xfrm><a:off x="` + tc.value + `" y="0"/><a:ext cx="100" cy="100"/></a:xfrm></p:spPr></p:sp></p:sld>`); e != nil {
			t.Fatal(e)
		}
		issues, e := slideBoundsIssuesChecked(d.Root(), 1000, 1000)
		if (e != nil) != tc.reject {
			t.Errorf("%q error %v", tc.value, e)
		}
		if !tc.reject && (len(issues) > 0) != tc.overflow {
			t.Errorf("%q issues %v", tc.value, issues)
		}
	}
}
