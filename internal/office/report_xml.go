package office

import (
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// xmlString serializes inspection snippets like ElementTree.tostring(...,
// encoding="unicode"). Resolve inherited namespaces before detaching anything;
// source prefixes and declarations are not the expanded names read by Reference.
// This is for reports only, not for rewriting preserved package members.
func xmlString(root *etree.Element) string {
	namespaces := map[string]string{}
	registered := map[string]string{
		"http://www.w3.org/XML/1998/namespace":        "xml",
		"http://www.w3.org/1999/xhtml":                "html",
		"http://www.w3.org/1999/02/22-rdf-syntax-ns#": "rdf",
		"http://schemas.xmlsoap.org/wsdl/":            "wsdl",
		"http://www.w3.org/2001/XMLSchema":            "xs",
		"http://www.w3.org/2001/XMLSchema-instance":   "xsi",
		"http://purl.org/dc/elements/1.1/":            "dc",
	}
	name := func(uri, local string) string {
		if uri == "" {
			return local
		}
		prefix, ok := namespaces[uri]
		if !ok {
			prefix = registered[uri]
			if prefix == "" {
				prefix = "ns" + strconv.Itoa(len(namespaces))
			}
			if prefix != "xml" {
				namespaces[uri] = prefix
			}
		}
		return prefix + ":" + local
	}
	elementURI := func(e *etree.Element) string {
		if e.Space == "xml" {
			return "http://www.w3.org/XML/1998/namespace"
		}
		return e.NamespaceURI()
	}
	attributeURI := func(a etree.Attr) string {
		if a.Space == "xml" {
			return "http://www.w3.org/XML/1998/namespace"
		}
		return a.NamespaceURI()
	}
	declaration := func(a etree.Attr) bool { return a.Space == "xmlns" || (a.Space == "" && a.Key == "xmlns") }
	var collect func(*etree.Element)
	collect = func(e *etree.Element) {
		name(elementURI(e), e.Tag)
		for _, a := range e.Attr {
			if !declaration(a) {
				name(attributeURI(a), a.Key)
			}
		}
		for _, c := range e.ChildElements() {
			collect(c)
		}
	}
	collect(root)
	var uris []string
	for uri := range namespaces {
		uris = append(uris, uri)
	}
	sort.Slice(uris, func(i, j int) bool { return namespaces[uris[i]] < namespaces[uris[j]] })
	textEscape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attrEscape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "\r", "&#13;", "\n", "&#10;", "\t", "&#09;")
	var out strings.Builder
	var write func(*etree.Element, bool)
	write = func(e *etree.Element, top bool) {
		tag := name(elementURI(e), e.Tag)
		out.WriteByte('<')
		out.WriteString(tag)
		if top {
			for _, uri := range uris {
				out.WriteString(" xmlns:")
				out.WriteString(namespaces[uri])
				out.WriteString("=\"")
				out.WriteString(attrEscape.Replace(uri))
				out.WriteByte('"')
			}
		}
		for _, a := range e.Attr {
			if !declaration(a) {
				out.WriteByte(' ')
				out.WriteString(name(attributeURI(a), a.Key))
				out.WriteString("=\"")
				out.WriteString(attrEscape.Replace(a.Value))
				out.WriteByte('"')
			}
		}
		hasContent := false
		for _, token := range e.Child {
			switch n := token.(type) {
			case *etree.Element:
				hasContent = true
			case *etree.CharData:
				hasContent = hasContent || n.Data != ""
			}
		}
		if !hasContent {
			out.WriteString(" />")
			return
		}
		out.WriteByte('>')
		// ElementTree's default parser drops comments/PIs and treats CDATA as text.
		for _, token := range e.Child {
			switch n := token.(type) {
			case *etree.Element:
				write(n, false)
			case *etree.CharData:
				out.WriteString(textEscape.Replace(n.Data))
			}
		}
		out.WriteString("</")
		out.WriteString(tag)
		out.WriteByte('>')
	}
	write(root, true)
	if parent := root.Parent(); parent != nil {
		found := false
		for _, token := range parent.Child {
			if token == root {
				found = true
				continue
			}
			if !found {
				continue
			}
			if _, ok := token.(*etree.Element); ok {
				break
			}
			if c, ok := token.(*etree.CharData); ok {
				out.WriteString(textEscape.Replace(c.Data))
			}
		}
	}
	return out.String()
}
