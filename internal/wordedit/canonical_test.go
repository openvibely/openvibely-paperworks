package wordedit

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// referenceCanonicalParse supplies XML 1.0 end-of-line and literal attribute
// whitespace normalization before etree expands character references. This is
// deliberately limited to the UTF-8, DTD-free privacy fixture, not a general
// replacement for libxml2. UNRUN source-derived parser contract.
func referenceCanonicalParse(t *testing.T, input string) *etree.Element {
	t.Helper()
	input = strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n")
	data := []byte(input)
	for i := 0; i < len(data); {
		if data[i] != '<' {
			i++
			continue
		}
		end := ""
		switch {
		case strings.HasPrefix(input[i:], "<!--"):
			end = "-->"
		case strings.HasPrefix(input[i:], "<![CDATA["):
			end = "]]>"
		case strings.HasPrefix(input[i:], "<?"):
			end = "?>"
		case strings.HasPrefix(input[i:], "<!"):
			t.Fatal("DTD/declarations outside canonical privacy fixture contract")
		}
		if end != "" {
			n := strings.Index(input[i:], end)
			if n < 0 {
				t.Fatal("unterminated canonical XML markup")
			}
			i += n + len(end)
			continue
		}
		i++
		var quote byte
		for i < len(data) {
			c := data[i]
			i++
			if quote != 0 {
				if c == quote {
					quote = 0
				} else if c == '\t' || c == '\n' {
					data[i-1] = ' '
				}
			} else if c == '\'' || c == '"' {
				quote = c
			} else if c == '>' {
				break
			}
		}
	}
	d := etree.NewDocument()
	d.ReadSettings.ValidateInput = true
	if err := d.ReadFromBytes(data); err != nil {
		t.Fatal(err)
	}
	if d.Root() == nil {
		t.Fatal("missing canonical XML root")
	}
	return d.Root()
}

// referenceCanonicalRoot implements inclusive C14N 1.0 with comments for the
// complete parsed, DTD-free roots in the privacy fixture. It does not implement
// arbitrary XPath node sets or DTD default attributes. UNRUN source port.
// https://www.w3.org/TR/2001/REC-xml-c14n-20010315#ProcessingModel
func referenceCanonicalRoot(t *testing.T, root *etree.Element) string {
	t.Helper()
	text := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\r", "&#xD;")
	attr := strings.NewReplacer("&", "&amp;", "<", "&lt;", "\"", "&quot;", "\t", "&#x9;", "\n", "&#xA;", "\r", "&#xD;")
	qname := func(prefix, local string) string {
		if prefix == "" {
			return local
		}
		return prefix + ":" + local
	}
	var b strings.Builder
	var visit func(*etree.Element, map[string]string)
	visit = func(e *etree.Element, inherited map[string]string) {
		if err := referenceCanonicalNamespaceURIs(e); err != nil {
			t.Fatal(err)
		}
		namespaces := make(map[string]string, len(inherited))
		for k, v := range inherited {
			namespaces[k] = v
		}
		var attrs []etree.Attr
		var prefixes []string
		for _, a := range e.Attr {
			if a.Space == "xmlns" || a.Space == "" && a.Key == "xmlns" {
				prefix := a.Key
				if a.Space == "" {
					prefix = ""
				}
				if namespaces[prefix] != a.Value {
					prefixes = append(prefixes, prefix)
				}
				namespaces[prefix] = a.Value
			} else {
				attrs = append(attrs, a)
			}
		}
		if err := referenceCanonicalExpandedNames(e, namespaces); err != nil {
			t.Fatal(err)
		}
		name := qname(e.Space, e.Tag)
		b.WriteString("<" + name)
		sort.Strings(prefixes)
		for _, prefix := range prefixes {
			b.WriteString(" xmlns")
			if prefix != "" {
				b.WriteString(":" + prefix)
			}
			b.WriteString("=\"" + attr.Replace(namespaces[prefix]) + "\"")
		}
		uri := func(a etree.Attr) string {
			if a.Space == "" {
				return ""
			}
			value, ok := namespaces[a.Space]
			if !ok {
				t.Fatalf("unbound canonical attribute prefix %q", a.Space)
			}
			return value
		}
		sort.Slice(attrs, func(i, j int) bool {
			u, v := uri(attrs[i]), uri(attrs[j])
			if u != v {
				return u < v
			}
			return attrs[i].Key < attrs[j].Key
		})
		for _, a := range attrs {
			b.WriteString(" " + qname(a.Space, a.Key) + "=\"" + attr.Replace(a.Value) + "\"")
		}
		b.WriteString(">")
		for _, token := range e.Child {
			switch n := token.(type) {
			case *etree.Element:
				visit(n, namespaces)
			case *etree.CharData:
				b.WriteString(text.Replace(n.Data))
			case *etree.Comment:
				b.WriteString("<!--" + strings.ReplaceAll(n.Data, "\r", "&#xD;") + "-->")
			case *etree.ProcInst:
				b.WriteString("<?" + n.Target)
				if n.Inst != "" {
					b.WriteString(" " + strings.ReplaceAll(n.Inst, "\r", "&#xD;"))
				}
				b.WriteString("?>")
			default:
				t.Fatalf("unsupported canonical XML token %T", token)
			}
		}
		b.WriteString("</" + name + ">")
	}
	if root == nil {
		t.Fatal("missing canonical root")
	}
	visit(root, map[string]string{"": "", "xml": "http://www.w3.org/XML/1998/namespace"})
	return b.String()
}

// UNRUN explicit canonical output vectors; no production XML comparator used.
func TestReferenceCanonicalRootContract(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"attribute order and empty tags", `<r z="2" a="1"><x/></r>`, `<r a="1" z="2"><x></x></r>`},
		{"inclusive unused namespace", `<r xmlns:z="urn:z" xmlns:a="urn:a" z:x="2" a:y="1" b="0"/>`, `<r xmlns:a="urn:a" xmlns:z="urn:z" b="0" a:y="1" z:x="2"></r>`},
		{"redundant namespace", `<r xmlns:p="urn:p"><p:x xmlns:p="urn:p"/></r>`, `<r xmlns:p="urn:p"><p:x></p:x></r>`},
		{"default reset and prefix rebind", `<r xmlns="urn:r" xmlns:p="urn:p"><x xmlns="" xmlns:p="urn:q"><p:y/></x></r>`, `<r xmlns="urn:r" xmlns:p="urn:p"><x xmlns="" xmlns:p="urn:q"><p:y></p:y></x></r>`},
		{"comments and processing instructions", `<r><!--keep--><?go now?><?empty?></r>`, `<r><!--keep--><?go now?><?empty?></r>`},
		{"escaping and CDATA", "<r a=\"&#x9;&#xA;&#xD;&quot;&amp;&lt;\"><![CDATA[<&>]]>&#xD;</r>", `<r a="&#x9;&#xA;&#xD;&quot;&amp;&lt;">&lt;&amp;&gt;&#xD;</r>`},
		{"prefix identity", `<q:r xmlns:q="urn:r"/>`, `<q:r xmlns:q="urn:r"></q:r>`},
		{"whitespace retained", "<r> \n<x/>\t</r>", "<r> \n<x></x>\t</r>"},
		{"literal attribute whitespace", "<r a=\"x\t\r\ny\rz\n\"/>", `<r a="x  y z "></r>`},
		{"references retain attribute whitespace", `<r a="x&#9;&#10;&#13;y"/>`, `<r a="x&#x9;&#xA;&#xD;y"></r>`},
		{"single quotes and greater than", "<r a='>\t' b=\"&quot;\n\"/>", `<r a="> " b="&quot; "></r>`},
		{"line endings in text and CDATA", "<r>a\r\nb\r<![CDATA[c\r\nd\r]]>&#13;</r>", "<r>a\nb\nc\nd\n&#xD;</r>"},
		{"markup whitespace is not attribute whitespace", "<r><!--a='\t\r\n'--><?pi a='\t\r'?></r>", "<r><!--a='\t\n'--><?pi a='\t\n'?></r>"},
		{"CDATA markup remains text", "<r><![CDATA[<x a='\t\n'/>]]></r>", "<r>&lt;x a='\t\n'/&gt;</r>"},
		{"xml namespace implicit", `<r xml:space="preserve"/>`, `<r xml:space="preserve"></r>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			referenceEqual(t, referenceCanonicalRoot(t, referenceCanonicalParse(t, tc.input)), tc.want)
		})
	}
}

// UNRUN: libxml2 c14n.c rejects nonempty namespace declarations without a
// URI scheme, even when unused. General libxml URI grammar equivalence is not
// claimed; these checks cover the ASCII declarations used by this fixture.
func referenceCanonicalNamespaceURIs(e *etree.Element) error {
	for _, a := range e.Attr {
		if !(a.Space == "xmlns" || a.Space == "" && a.Key == "xmlns") || a.Value == "" {
			continue
		}
		u, err := url.Parse(a.Value)
		if err != nil || u.Scheme == "" {
			return fmt.Errorf("invalid or relative canonical namespace URI %q", a.Value)
		}
	}
	return nil
}

func TestReferenceCanonicalNamespaceURIContract(t *testing.T) {
	// UNRUN source-derived rejection/acceptance cases, not execution evidence.
	for _, tc := range []struct {
		name, input string
		wantError   bool
	}{
		{"relative default", `<r xmlns="relative"/>`, true},
		{"unused relative prefix", `<r xmlns:p="../relative"/>`, true},
		{"network path", `<r xmlns:p="//example.org/ns"/>`, true},
		{"fragment only", `<r xmlns:p="#ns"/>`, true},
		{"absolute URN", `<r xmlns:p="urn:fixture"/>`, false},
		{"absolute HTTPS", `<r xmlns="https://example.org/ns"/>`, false},
		{"default reset", `<r xmlns=""/>`, false},
		{"ordinary attribute is not namespace", `<r href="relative"/>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := referenceCanonicalParse(t, tc.input)
			referenceEqual(t, referenceCanonicalNamespaceURIs(root) != nil, tc.wantError)
		})
	}
}

// UNRUN: validate expanded names before sorting, including singleton attributes.
// Default namespaces apply to elements, never to unprefixed attributes.
func referenceCanonicalExpandedNames(e *etree.Element, namespaces map[string]string) error {
	if e.Space != "" && namespaces[e.Space] == "" {
		return fmt.Errorf("unbound canonical element prefix %q", e.Space)
	}
	seen := map[[2]string]bool{}
	for _, a := range e.Attr {
		if a.Space == "xmlns" || a.Space == "" && a.Key == "xmlns" {
			continue
		}
		uri := ""
		if a.Space != "" {
			uri = namespaces[a.Space]
			if uri == "" {
				return fmt.Errorf("unbound canonical attribute prefix %q", a.Space)
			}
		}
		key := [2]string{uri, a.Key}
		if seen[key] {
			return fmt.Errorf("duplicate canonical expanded attribute %v", key)
		}
		seen[key] = true
	}
	return nil
}

func TestReferenceCanonicalExpandedNamesContract(t *testing.T) {
	// Construct nodes directly: rejection must not depend on etree parser behavior.
	for _, tc := range []struct {
		name, element string
		attrs         [][2]string
		wantError     bool
	}{
		{"unknown element", "q:r", nil, true},
		{"unknown singleton attribute", "r", [][2]string{{"q:x", "1"}}, true},
		{"alias collision", "r", [][2]string{{"p:x", "1"}, {"alias:x", "2"}}, true},
		{"distinct local names", "p:r", [][2]string{{"p:x", "1"}, {"alias:y", "2"}}, false},
		{"default does not qualify attributes", "r", [][2]string{{"x", "1"}, {"p:x", "2"}}, false},
		{"implicit xml binding", "r", [][2]string{{"xml:space", "preserve"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := etree.NewElement(tc.element)
			for _, a := range tc.attrs {
				e.CreateAttr(a[0], a[1])
			}
			ns := map[string]string{"": "urn:p", "p": "urn:p", "alias": "urn:p", "xml": "http://www.w3.org/XML/1998/namespace"}
			referenceEqual(t, referenceCanonicalExpandedNames(e, ns) != nil, tc.wantError)
		})
	}
}
