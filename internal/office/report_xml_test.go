package office

import (
	"github.com/beevik/etree"
	"testing"
)

// Expected strings captured from pinned Reference's stdlib ElementTree default
// parser/tostring. These cover report serialization, not all parser semantics.
func TestReportXMLMatchesElementTree(t *testing.T) {
	for _, c := range []struct{ name, input, want string }{
		{"position", "<r xmlns:wp=\"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing\"><wp:positionH relativeFrom=\"column\"><wp:posOffset>914400</wp:posOffset></wp:positionH></r>", "<ns0:positionH xmlns:ns0=\"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing\" relativeFrom=\"column\"><ns0:posOffset>914400</ns0:posOffset></ns0:positionH>"},
		{"inherited-attributes", "<r xmlns:p=\"urn:p\" xmlns:a=\"urn:a\"><p:item a:z=\"1\" k=\"2\"><a:child/></p:item></r>", "<ns0:item xmlns:ns0=\"urn:p\" xmlns:ns1=\"urn:a\" ns1:z=\"1\" k=\"2\"><ns1:child /></ns0:item>"},
		{"shadowing", "<r xmlns:a=\"urn:a\"><a:one><a:two xmlns:a=\"urn:b\"/></a:one></r>", "<ns0:one xmlns:ns0=\"urn:a\" xmlns:ns1=\"urn:b\"><ns1:two /></ns0:one>"},
		{"xml-reserved", "<r xmlns:a=\"urn:a\"><a:p xml:space=\"preserve\" xml:lang=\"en\"> a </a:p></r>", "<ns0:p xmlns:ns0=\"urn:a\" xml:space=\"preserve\" xml:lang=\"en\"> a </ns0:p>"},
		{"registered", "<r xmlns:x=\"http://www.w3.org/1999/xhtml\" xmlns:z=\"urn:z\"><x:p z:a=\"1\"/></r>", "<html:p xmlns:html=\"http://www.w3.org/1999/xhtml\" xmlns:ns1=\"urn:z\" ns1:a=\"1\" />"},
		{"escaping", "<r><p a=\"&quot;&amp;&lt;&gt;'&#13;&#10;&#9;\">&amp;&lt;&gt;'é</p></r>", "<p a=\"&quot;&amp;&lt;&gt;'&#13;&#10;&#09;\">&amp;&lt;&gt;'é</p>"},
		{"tail", "<r><p/>x&amp;y<!-- ignored -->after<?pi data?>end<q/></r>", "<p />x&amp;yafterend"},
		{"cdata", "<r><p><![CDATA[a<b&c>]]><!-- ignored -->tail<?pi data?></p></r>", "<p>a&lt;b&amp;c&gt;tail</p>"},
		{"empty-comments", "<r><p><!-- ignored --><?pi data?></p></r>", "<p />"},
		{"default-namespace", "<r xmlns=\"urn:r\"><p><q xmlns=\"\"/></p></r>", "<ns0:p xmlns:ns0=\"urn:r\"><q /></ns0:p>"},
		{"default-not-attribute", "<r xmlns=\"urn:r\"><p a=\"x\"/></r>", "<ns0:p xmlns:ns0=\"urn:r\" a=\"x\" />"},
		{"prefix-lexical-order", "<r><p><x:a xmlns:x=\"urn:0\"/><x:a xmlns:x=\"urn:1\"/><x:a xmlns:x=\"urn:2\"/><x:a xmlns:x=\"urn:3\"/><x:a xmlns:x=\"urn:4\"/><x:a xmlns:x=\"urn:5\"/><x:a xmlns:x=\"urn:6\"/><x:a xmlns:x=\"urn:7\"/><x:a xmlns:x=\"urn:8\"/><x:a xmlns:x=\"urn:9\"/><x:a xmlns:x=\"urn:10\"/><x:a xmlns:x=\"urn:11\"/></p></r>", "<p xmlns:ns0=\"urn:0\" xmlns:ns1=\"urn:1\" xmlns:ns10=\"urn:10\" xmlns:ns11=\"urn:11\" xmlns:ns2=\"urn:2\" xmlns:ns3=\"urn:3\" xmlns:ns4=\"urn:4\" xmlns:ns5=\"urn:5\" xmlns:ns6=\"urn:6\" xmlns:ns7=\"urn:7\" xmlns:ns8=\"urn:8\" xmlns:ns9=\"urn:9\"><ns0:a /><ns1:a /><ns2:a /><ns3:a /><ns4:a /><ns5:a /><ns6:a /><ns7:a /><ns8:a /><ns9:a /><ns10:a /><ns11:a /></p>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := etree.NewDocument()
			if e := d.ReadFromString(c.input); e != nil {
				t.Fatal(e)
			}
			before, e := d.WriteToString()
			if e != nil {
				t.Fatal(e)
			}
			got := xmlString(d.Root().ChildElements()[0])
			if got != c.want {
				t.Fatalf("%q != %q", got, c.want)
			}
			after, e := d.WriteToString()
			if e != nil || before != after {
				t.Fatal("serializer mutated source", e)
			}
		})
	}
}

// ElementTree preserves both unqualified and implicitly xml-qualified names.
// Collapsing these loses information when both attributes have the same local name.
func TestReportAttributesImplicitXMLNamespace(t *testing.T) {
	d := etree.NewDocument()
	if err := d.ReadFromString(`<r xmlns="urn:default" xmlns:a="urn:a"><p space="ordinary" xml:space="preserve" xml:lang="en" a:space="custom"/></r>`); err != nil {
		t.Fatal(err)
	}
	got := reportAttrMap(d.Root().ChildElements()[0])
	want := map[string]string{"space": "ordinary", "{http://www.w3.org/XML/1998/namespace}space": "preserve", "{http://www.w3.org/XML/1998/namespace}lang": "en", "{urn:a}space": "custom"}
	if len(got) != len(want) {
		t.Fatalf("attributes %#v want %#v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: %q want %q", k, got[k], v)
		}
	}
}
