package office

import (
	"github.com/beevik/etree"
	"testing"
)

// Expected bytes freshly regenerated with pinned Reference minidom.toxml.
func TestFormulaCacheXMLDeclaration(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"bare", "<s><v>7</v></s>", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><s><v>7</v></s>"},
		{"standalone", "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n<s/>", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><s/>"},
		{"outside-whitespace", " \n<s>\n <v/>\n</s>\n", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><s>\n <v/>\n</s>"},
		{"comments-pi", "<?xml version=\"1.0\"?><!--before--><?mode x?><s/><!--after-->", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><!--before--><?mode x?><s/><!--after-->"},
		{"unicode", "<s label=\"\u00e9 &amp; &quot;\">r\u00e9sum\u00e9</s>", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><s label=\"\u00e9 &amp; &quot;\">r\u00e9sum\u00e9</s>"},
		{"bom", "\ufeff<?xml version=\"1.0\"?><s/>", "<?xml version=\"1.0\" encoding=\"UTF-8\"?><s/>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := etree.NewDocument()
			if e := d.ReadFromString(tc.source); e != nil {
				t.Fatal(e)
			}
			before, e := d.WriteToString()
			if e != nil {
				t.Fatal(e)
			}
			got, e := formulaCacheXML(d)
			if e != nil || string(got) != tc.want {
				t.Fatalf("%q, %v; want %q", got, e, tc.want)
			}
			after, e := d.WriteToString()
			if e != nil || before != after {
				t.Fatal("source mutated", e)
			}
		})
	}
}
