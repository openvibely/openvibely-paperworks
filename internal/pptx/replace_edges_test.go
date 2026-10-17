package pptx

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func TestReplaceRunMatchBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		parts       []string
		match, text string
		occurrence  int
		want        []string
		fail        bool
	}{
		{"empty middle", []string{"ab", "", "cd"}, "bc", "X", 0, []string{"aX", "", "d"}, false},
		{"unicode match", []string{"前😀", "後終"}, "😀後", "&新", 0, []string{"前&新", "終"}, false},
		{"nonoverlapping occurrence", []string{"aa", "aa"}, "aa", "X", 1, []string{"aa", "X"}, false},
		{"no overlapping occurrence", []string{"a", "aa"}, "aa", "X", 1, nil, true},
		{"break boundary", []string{"ab", "<break>", "cd"}, "bc", "X", 0, nil, true},
		{"field boundary", []string{"ab", "<field>", "cd"}, "bc", "X", 0, nil, true},
		{"occurrence after break", []string{"ab", "<break>", "ab"}, "ab", "X", 1, []string{"ab", "X"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := etree.NewDocument()
			p := d.CreateElement("a:p")
			p.CreateAttr("xmlns:a", ans)
			for _, s := range tc.parts {
				switch s {
				case "<break>":
					p.CreateElement("a:br")
				case "<field>":
					p.CreateElement("a:fld").CreateElement("a:t").SetText("field")
				default:
					p.CreateElement("a:r").CreateElement("a:t").SetText(s)
				}
			}
			before, _ := d.WriteToString()
			err := replaceRunMatch(context.Background(), p, tc.match, tc.text, tc.occurrence)
			if tc.fail {
				after, _ := d.WriteToString()
				if err == nil || before != after {
					t.Fatal("failed match mutated paragraph", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range office.Children(p, office.NSA, "r") {
				got = append(got, office.Child(r, office.NSA, "t").Text())
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatal(got, tc.want)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := replaceRunMatch(ctx, p, "a", "x", 0); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
