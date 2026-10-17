package office

import (
	"archive/zip"
	"bytes"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/beevik/etree"
)

// Test-side counterpart of document_native.chart_parts. Read declarations even
// when their named member is absent; using production ChartParts for acquisition
// would otherwise let a discovery regression hide an invalid saved preview.
func referencePreviewChartParts(t *testing.T, z *zip.ReadCloser) []string {
	t.Helper()
	names := map[string]bool{}
	for _, member := range z.File {
		names[member.Name] = true
	}
	read := func(name string) *etree.Element {
		t.Helper()
		d := etree.NewDocument()
		if err := referencePreviewReadXML(d, referencePreviewZIPRead(t, z, name)); err != nil {
			t.Fatal(name, err)
		}
		if d.Root() == nil {
			t.Fatal("empty XML", name)
		}
		return d.Root()
	}
	mime := func(value string) bool {
		return value == "application/vnd.openxmlformats-officedocument.drawingml.chart+xml" || value == "application/vnd.ms-office.chartex+xml"
	}
	found := map[string]bool{}
	defaults := map[string]string{}
	if names["[Content_Types].xml"] {
		for _, element := range read("[Content_Types].xml").ChildElements() {
			if Is(element, "http://schemas.openxmlformats.org/package/2006/content-types", "Default") {
				defaults[element.SelectAttrValue("Extension", "")] = element.SelectAttrValue("ContentType", "")
			}
			if part := element.SelectAttrValue("PartName", ""); part != "" && mime(element.SelectAttrValue("ContentType", "")) {
				found[strings.TrimLeft(part, "/")] = true
			}
		}
	}
	for _, member := range z.File {
		name := member.Name
		extension := name[strings.LastIndex(name, ".")+1:]
		if mime(defaults[extension]) {
			found[name] = true
		}
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		owner := path.Join(path.Dir(path.Dir(name)), strings.TrimSuffix(path.Base(name), ".rels"))
		for _, rel := range read(name).ChildElements() {
			kind := rel.SelectAttrValue("Type", "")
			kind = kind[strings.LastIndex(kind, "/")+1:]
			if kind != "chart" && kind != "chartEx" {
				continue
			}
			if rel.SelectAttrValue("TargetMode", "") == "External" {
				t.Fatal("external chart part is unsupported")
			}
			target := referencePreviewUnquote(rel.SelectAttrValue("Target", ""))
			target, _, _ = strings.Cut(target, "#")
			resolved := target
			if !strings.HasPrefix(target, "/") {
				resolved = path.Join(path.Dir(owner), target)
			}
			resolved = strings.TrimLeft(path.Clean(resolved), "/")
			if target == "" || strings.HasPrefix(resolved, "../") {
				t.Fatal("invalid package relationship target", target)
			}
			found[resolved] = true
		}
	}
	var result []string
	for name := range found {
		if !names[name] {
			t.Fatal("missing chart part", name)
		}
		root := read(name)
		if !Is(root, NSC, "chartSpace") && !Is(root, NSCX, "chartSpace") {
			t.Fatal("invalid chart part", name)
		}
		result = append(result, name)
	}
	slices.SortStableFunc(result, referencePreviewNaturalCompare)
	return result
}

// referencePreviewUnquote mirrors urllib.parse.unquote's default UTF-8/replacement
// policy. Invalid percent escapes remain literal; '+' is not form-decoded.
// Decode invalid UTF-8 by maximal valid prefix, not by collapsing adjacent errors.
func referencePreviewUnquote(value string) string {
	if !strings.Contains(value, "%") {
		return value
	}
	hex := func(b byte) int {
		switch {
		case b >= '0' && b <= '9':
			return int(b - '0')
		case b >= 'a' && b <= 'f':
			return int(b-'a') + 10
		case b >= 'A' && b <= 'F':
			return int(b-'A') + 10
		}
		return -1
	}
	decoded := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '%' && i+2 < len(value) {
			a, b := hex(value[i+1]), hex(value[i+2])
			if a >= 0 && b >= 0 {
				decoded = append(decoded, byte(a*16+b))
				i += 2
				continue
			}
		}
		decoded = append(decoded, value[i])
	}
	var result strings.Builder
	for i := 0; i < len(decoded); {
		r, size := utf8.DecodeRune(decoded[i:])
		if r != utf8.RuneError || size != 1 {
			result.Write(decoded[i : i+size])
			i += size
			continue
		}
		// A legal lead followed by a valid but incomplete prefix consumes that
		// prefix as one error. An illegal lead consumes only itself.
		lead, need := decoded[i], 1
		switch {
		case lead >= 0xC2 && lead <= 0xDF:
			need = 2
		case lead >= 0xE0 && lead <= 0xEF:
			need = 3
		case lead >= 0xF0 && lead <= 0xF4:
			need = 4
		}
		consumed := 1
		for consumed < need && i+consumed < len(decoded) {
			b := decoded[i+consumed]
			if b < 0x80 || b > 0xBF {
				break
			}
			if consumed == 1 && ((lead == 0xE0 && b < 0xA0) ||
				(lead == 0xED && b > 0x9F) || (lead == 0xF0 && b < 0x90) ||
				(lead == 0xF4 && b > 0x8F)) {
				break
			}
			consumed++
		}
		result.WriteRune(utf8.RuneError)
		i += consumed
	}
	return result.String()
}

// Supplemental decoder contracts, UNRUN. These do not add original review units.
func TestReferencePreviewUnquoteContract(t *testing.T) {
	for _, c := range []struct{ input, want string }{
		{"../charts/chart1.xml", "../charts/chart1.xml"},
		{"a+b%20c", "a+b c"},
		{"%", "%"}, {"%2", "%2"}, {"%GG%41", "%GGA"},
		{"%252F", "%2F"}, {"%23fragment", "#fragment"},
		{"%E2%82%AC", "€"}, {"%F0%9F%98%80", "😀"},
		{"%FF%FE", "��"}, {"%E2%82", "�"}, {"%E2%82X", "�X"},
		{"%E0%80%80", "���"}, {"%ED%A0%80", "���"},
		{"%F4%90%80%80", "����"}, {"%F0%9F%98", "�"},
		{"%EF%BF%BD", "�"},
		{"é%E2%82€%AC", "é�€�"}, {"%C2é%A2", "�é�"},
		{"%E2%82%41", "�A"}, {"%F0%90%80X", "�X"},
		{"%C0%AF", "��"}, {"%F5%80%80%80", "����"},
		{"%E0%A0%80", "\u0800"}, {"%ED%9F%BF", "\ud7ff"},
		{"%F4%8F%BF%BF", "\U0010ffff"}, {"%00", "\x00"},
		{"%%41", "%A"}, {"%2é%41", "%2éA"},
	} {
		if got := referencePreviewUnquote(c.input); got != c.want {
			t.Fatalf("unquote(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// Match re.split(r'(\d+)', name): keep the empty leading/trailing text
// segments and compare decimal runs as arbitrary-sized nonnegative integers.
// Reference's set iteration leaves equal-key order unspecified; stable sorting must
// not introduce a lexical tie-break for chart1 versus chart01.
var referencePreviewDecimalRun = regexp.MustCompile(`\p{Nd}+`)

func referencePreviewNaturalCompare(a, b string) int {
	key := func(s string) []string {
		var parts []string
		end := 0
		for _, span := range referencePreviewDecimalRun.FindAllStringIndex(s, -1) {
			parts = append(parts, s[end:span[0]])
			var digits strings.Builder
			for _, r := range s[span[0]:span[1]] {
				// Unicode Nd ranges consist of ordered decimal digit sets.
				for _, rr := range unicode.Digit.R16 {
					if uint32(r) >= uint32(rr.Lo) && uint32(r) <= uint32(rr.Hi) && (uint32(r)-uint32(rr.Lo))%uint32(rr.Stride) == 0 {
						digits.WriteByte('0' + byte((uint32(r)-uint32(rr.Lo))/uint32(rr.Stride)%10))
						break
					}
				}
				for _, rr := range unicode.Digit.R32 {
					if uint32(r) >= rr.Lo && uint32(r) <= rr.Hi && (uint32(r)-rr.Lo)%rr.Stride == 0 {
						digits.WriteByte('0' + byte((uint32(r)-rr.Lo)/rr.Stride%10))
						break
					}
				}
			}
			parts = append(parts, strings.TrimLeft(digits.String(), "0"))
			end = span[1]
		}
		return append(parts, s[end:])
	}
	left, right := key(a), key(b)
	for i := 0; i < len(left) && i < len(right); i++ {
		if i%2 == 1 && len(left[i]) != len(right[i]) {
			if len(left[i]) < len(right[i]) {
				return -1
			}
			return 1
		}
		if c := strings.Compare(left[i], right[i]); c != 0 {
			return c
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

// Supplemental acquisition contracts, UNRUN; not original review units.
func TestReferencePreviewNaturalCompareContract(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"chart2.xml", "chart10.xml", -1},
		{"chart01.xml", "chart1.xml", 0},
		{"chart000.xml", "chart0.xml", 0},
		{"chart99999999999999999999.xml", "chart100000000000000000000.xml", -1},
		{"1chart", "chart", -1},
		{"chart", "chart0", -1},
		{"chart1", "chart1a", -1},
		{"chart1part10", "chart1part2", 1},
		{"Chart1", "chart1", -1},
		{"chart١٢.xml", "chart12.xml", 0},
		{"chart１２.xml", "chart12.xml", 0},
		{"chart𝟙𝟚.xml", "chart12.xml", 0},
	} {
		got := referencePreviewNaturalCompare(c.a, c.b)
		if got < 0 {
			got = -1
		} else if got > 0 {
			got = 1
		}
		if got != c.want {
			t.Fatalf("compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	names := []string{"chart01", "chart1", "chart001"}
	slices.SortStableFunc(names, referencePreviewNaturalCompare)
	if !slices.Equal(names, []string{"chart01", "chart1", "chart001"}) {
		t.Fatal("equal numeric keys reordered", names)
	}
}

// Preserve in-scope namespace bindings when copying a subtree for comparison.
// etree.Copy detaches its parent; lxml tostring(element) includes inherited
// namespace declarations. This does not certify all lxml serialization semantics
// (notably namespace ordering and encoding). Preserve following text as well:
// lxml tostring defaults to with_tail=True; etree.Copy omits sibling tokens.
func referencePreviewNamespaceSnapshot(element *etree.Element) *etree.Document {
	copy := element.Copy()
	seen := map[string]bool{}
	for ancestor := element; ancestor != nil; ancestor = ancestor.Parent() {
		for _, attr := range ancestor.Attr {
			if attr.Space != "xmlns" && !(attr.Space == "" && attr.Key == "xmlns") {
				continue
			}
			name := attr.Key
			if attr.Space == "xmlns" {
				name = "xmlns:" + attr.Key
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			if ancestor != element {
				copy.CreateAttr(name, attr.Value)
			}
		}
	}
	doc := etree.NewDocument()
	doc.SetRoot(copy)
	copy.SetTail(element.Tail())
	return doc
}

// Supplemental namespace acquisition contract, UNRUN; no original review credit.
func TestReferencePreviewNamespaceSnapshotContract(t *testing.T) {
	doc := etree.NewDocument()
	if err := doc.ReadFromString(`<c:root xmlns:c="urn:chart" xmlns="urn:outer" xmlns:x="urn:old"><c:series xmlns:x="urn:new"><c:marker xmlns:y="urn:local"><x:point/></c:marker></c:series></c:root>`); err != nil {
		t.Fatal(err)
	}
	marker := doc.Root().ChildElements()[0].ChildElements()[0]
	snapshot := referencePreviewNamespaceSnapshot(marker).Root()
	for _, c := range []struct{ name, want string }{
		{"xmlns:c", "urn:chart"}, {"xmlns", "urn:outer"},
		{"xmlns:x", "urn:new"}, {"xmlns:y", "urn:local"},
	} {
		if got := snapshot.SelectAttrValue(c.name, ""); got != c.want {
			t.Fatalf("%s = %q, want %q", c.name, got, c.want)
		}
	}
	if snapshot.ChildElements()[0].NamespaceURI() != "urn:new" {
		t.Fatal("shadowed descendant namespace lost")
	}
	if len(marker.Attr) != 1 {
		t.Fatal("snapshot changed source declarations")
	}
	marker.Parent().CreateAttr("xmlns:x", "urn:changed")
	if referencePreviewNamespaceSnapshot(marker).Root().SelectAttrValue("xmlns:x", "") != "urn:changed" {
		t.Fatal("fresh snapshot omitted changed ancestor binding")
	}
	if snapshot.SelectAttrValue("xmlns:x", "") != "urn:new" {
		t.Fatal("original snapshot changed with ancestor")
	}
}

// Supplemental lxml with_tail=True acquisition cases, UNRUN. These do not
// certify byte serialization equivalence or add original review units.
func TestReferencePreviewNamespaceSnapshotTailContract(t *testing.T) {
	for _, c := range []struct{ xml, tail, escaped string }{
		{`<root><marker/></root>`, "", ""},
		{"<root><marker/> \n\t</root>", " \n\t", " \n\t"},
		{`<root><marker/>a&amp;b&lt;c<next/>excluded</root>`, "a&b<c", "a&amp;b&lt;c"},
		{`<root><marker/>first<!--stop-->excluded</root>`, "first", "first"},
		{`<root><marker/>first<?stop here?>excluded</root>`, "first", "first"},
		{`<root><marker/>a<![CDATA[<&]]>b<next/></root>`, "a<&b", "a&lt;&amp;b"},
	} {
		doc := etree.NewDocument()
		if err := doc.ReadFromString(c.xml); err != nil {
			t.Fatal(err)
		}
		marker := doc.Root().ChildElements()[0]
		snapshot := referencePreviewNamespaceSnapshot(marker)
		if got := snapshot.Root().Tail(); got != c.tail {
			t.Fatalf("%s: tail %q, want %q", c.xml, got, c.tail)
		}
		if len(snapshot.Child) != 1 && !(len(snapshot.Child) == 2 && c.tail != "") {
			t.Fatalf("%s: snapshot included non-tail siblings", c.xml)
		}
		encoded, err := snapshot.WriteToString()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(encoded, "/>"+c.escaped) {
			t.Fatalf("%s: escaped tail missing from %q", c.xml, encoded)
		}
		marker.SetTail("changed")
		if snapshot.Root().Tail() != c.tail {
			t.Fatal("source mutation changed snapshot tail")
		}
		if referencePreviewNamespaceSnapshot(marker).Root().Tail() != "changed" {
			t.Fatal("fresh snapshot omitted changed tail")
		}
	}
}

// Supplemental element-versus-document acquisition contract, UNRUN. lxml's
// tostring(_Element) excludes surrounding document markup but retains element
// comments, processing instructions and following character data.
func TestReferencePreviewNamespaceSnapshotDocumentScope(t *testing.T) {
	doc := etree.NewDocument()
	if err := doc.ReadFromString(`<?xml version="1.0"?><!--before--><?before value?><!DOCTYPE root><root xmlns:c="urn:chart"><!--inside--><?inside value?><c:marker/></root><!--after--><?after value?>`); err != nil {
		t.Fatal(err)
	}
	before, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := referencePreviewNamespaceSnapshot(doc.Root())
	encoded, err := snapshot.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	want := `<root xmlns:c="urn:chart"><!--inside--><?inside value?><c:marker/></root>`
	if encoded != want {
		t.Fatalf("element scope: got %q, want %q", encoded, want)
	}
	after, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("element snapshot changed source document")
	}
}

// Shared comparison boundary, UNRUN. Attribute and ordinary text escaping follows
// retained libxml2 xmlsave.c tables and encoding.c decimal-reference fallback.
// Namespace acquisition/order, parser normalization and original-runtime
// provenance remain pending. The final ASCII conversion is distinct from escaping.
func referencePreviewSerializeElement(element *etree.Element) ([]byte, error) {
	doc := referencePreviewNamespaceSnapshot(element)
	referencePreviewNamespaceAttributesFirst(doc.Root())
	var out bytes.Buffer
	var write func(etree.Token)
	write = func(token etree.Token) {
		switch node := token.(type) {
		case *etree.Element:
			out.WriteByte('<')
			out.WriteString(node.FullTag())
			for _, attr := range node.Attr {
				out.WriteByte(' ')
				out.WriteString(attr.FullKey())
				out.WriteString(`="`)
				referencePreviewEscapeValue(&out, attr.Value, true)
				out.WriteByte('"')
			}
			if len(node.Child) == 0 {
				out.WriteString("/>")
				return
			}
			out.WriteByte('>')
			for _, child := range node.Child {
				write(child)
			}
			out.WriteString("</")
			out.WriteString(node.FullTag())
			out.WriteByte('>')
		case *etree.CharData:
			if node.IsCData() {
				node.WriteTo(&out, &doc.WriteSettings)
			} else {
				referencePreviewEscapeValue(&out, node.Data, false)
			}
		default:
			token.WriteTo(&out, &doc.WriteSettings)
		}
	}
	for _, token := range doc.Child {
		write(token)
	}
	// libxml2 encoding.c converts the complete output buffer, not only XML
	// values. References in comments/CDATA are consequently literal content;
	// this is byte-serialization correspondence, not a value-preserving round trip.
	var ascii bytes.Buffer
	for _, r := range out.String() {
		if r > 127 {
			ascii.WriteString("&#")
			ascii.WriteString(strconv.FormatInt(int64(r), 10))
			ascii.WriteByte(';')
		} else {
			ascii.WriteRune(r)
		}
	}
	return ascii.Bytes(), nil
}

// Escape values before writing markup, never by replacing substrings in a
// serialized document. Literal reference-looking text is escaped exactly once.
func referencePreviewEscapeValue(out *bytes.Buffer, value string, attribute bool) {
	for _, r := range value {
		switch {
		case r == '&':
			out.WriteString("&amp;")
		case r == '<':
			out.WriteString("&lt;")
		case r == '>':
			out.WriteString("&gt;")
		case attribute && r == '"':
			out.WriteString("&quot;")
		case r > 127 || r == '\r' || (attribute && (r == '\n' || r == '\t')):
			out.WriteString("&#")
			out.WriteString(strconv.FormatInt(int64(r), 10))
			out.WriteByte(';')
		default:
			out.WriteRune(r)
		}
	}
}

// UNRUN gap contracts, not conformance evidence. ASCII encoding must retain
// Unicode values through XML references, including the serialized element tail.
// These cases use ASCII names and ordinary values. Output-encoder cases below
// separately cover tokens whose references do not preserve parsed values.
func TestReferencePreviewElementASCIIEncodingContract(t *testing.T) {
	for _, c := range []struct{ attribute, text, tail string }{
		{"plain", "<&>", " tail"},
		{"café", "", ""},
		{"", "é Ω 中 😀", ""},
		{"", "", "é<& 😀"},
		{"left\tright", "", ""},
		{"left\nright", "", ""},
		{"left\rright", "", ""},
		{"", "left\rright", ""},
		{"", "", "left\r\nright"},
		{"\"'<&>\t\n\r", "\"'<&>\r", "\"'<&>\r"},
	} {
		doc := etree.NewDocument()
		root := doc.CreateElement("root")
		marker := root.CreateElement("marker")
		marker.CreateAttr("label", c.attribute)
		marker.SetText(c.text)
		marker.SetTail(c.tail)
		before, err := doc.WriteToString()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := referencePreviewSerializeElement(marker)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range encoded {
			if b > 127 {
				t.Fatalf("lxml default ASCII encoding: non-ASCII byte in %q", encoded)
			}
		}
		if strings.Contains(string(encoded), "<?xml") {
			t.Fatalf("unexpected XML declaration: %q", encoded)
		}
		// A Go XML round trip alone is insufficient: encoding/xml preserves
		// literal attribute tabs/newlines that libxml2 normalizes to spaces.
		// Require references for those values before checking decoded values.
		start := strings.Index(string(encoded), `label="`)
		if start < 0 {
			t.Fatalf("serialized label missing: %q", encoded)
		}
		attribute := string(encoded)[start+len(`label="`):]
		end := strings.IndexByte(attribute, '"')
		if end < 0 {
			t.Fatalf("serialized label unterminated: %q", encoded)
		}
		if strings.ContainsAny(attribute[:end], "\t\n\r") {
			t.Fatalf("literal attribute whitespace would normalize in libxml2: %q", encoded)
		}
		if strings.ContainsRune(string(encoded), '\r') {
			t.Fatalf("literal carriage return would normalize on XML acquisition: %q", encoded)
		}
		// A wrapper permits a nonempty tail when reparsing the element.
		decoded := etree.NewDocument()
		if err := decoded.ReadFromString("<wrapper>" + string(encoded) + "</wrapper>"); err != nil {
			t.Fatal(err)
		}
		children := decoded.Root().ChildElements()
		if len(children) != 1 {
			t.Fatalf("serialized element count = %d", len(children))
		}
		got := children[0]
		if got.SelectAttrValue("label", "") != c.attribute || got.Text() != c.text || got.Tail() != c.tail {
			t.Fatalf("encoding lost attribute/text/tail: %q", encoded)
		}
		after, err := doc.WriteToString()
		if err != nil || after != before {
			t.Fatal("serialization changed source", err)
		}
	}
}

// libxml2 xmlNodeDumpOutputInternal writes nsDef before properties. etree stores
// both in Attr, so partition only the detached snapshot, recursively. This does
// not determine lxml's inherited nsDef allocation or declaration deduplication.
func referencePreviewNamespaceAttributesFirst(element *etree.Element) {
	isNamespace := func(a etree.Attr) bool {
		return a.Space == "xmlns" || (a.Space == "" && a.Key == "xmlns")
	}
	attributes := make([]etree.Attr, 0, len(element.Attr))
	for _, a := range element.Attr {
		if isNamespace(a) {
			attributes = append(attributes, a)
		}
	}
	for _, a := range element.Attr {
		if !isNamespace(a) {
			attributes = append(attributes, a)
		}
	}
	element.Attr = attributes
	for _, child := range element.ChildElements() {
		referencePreviewNamespaceAttributesFirst(child)
	}
}

// Supplemental source-derived ordering contracts, UNRUN; no original review
// credit. Keep relative declaration/attribute order, including on descendants.
func TestReferencePreviewNamespaceAttributeOrderContract(t *testing.T) {
	for _, c := range []struct{ input, want string }{
		{`<root z="1" xmlns:q="urn:q" a="2" xmlns:p="urn:p"/>`, `<root xmlns:q="urn:q" xmlns:p="urn:p" z="1" a="2"/>`},
		{`<root z="1" xmlns="urn:default" a="2"/>`, `<root xmlns="urn:default" z="1" a="2"/>`},
		{`<root><child z="1" xmlns:q="urn:q" q:a="2"/></root>`, `<root><child xmlns:q="urn:q" z="1" q:a="2"/></root>`},
		{`<root z="1" a="2"/>`, `<root z="1" a="2"/>`},
	} {
		doc := etree.NewDocument()
		if err := doc.ReadFromString(c.input); err != nil {
			t.Fatal(err)
		}
		before, err := doc.WriteToString()
		if err != nil {
			t.Fatal(err)
		}
		got, err := referencePreviewSerializeElement(doc.Root())
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Fatalf("namespace/attribute order: got %q, want %q", got, c.want)
		}
		after, err := doc.WriteToString()
		if err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatal("serialization reordered source attributes")
		}
	}
}

// UNRUN source-derived byte contracts: a value-preserving XML round trip cannot
// distinguish libxml2's decimal references from etree's hexadecimal references.
// Keep these failures visible until the complete serialization adapter is ported.
func TestReferencePreviewElementReferenceSpellingContract(t *testing.T) {
	for _, c := range []struct{ attribute, text, tail, want string }{
		{"\t\n\r", "", "", `<marker label="&#9;&#10;&#13;"/>`},
		{"", "\r", "", `<marker label="">&#13;</marker>`},
		{"", "", "\r", `<marker label=""/>&#13;`},
		{"\"'<&>", "\"'<&>", "", `<marker label="&quot;'&lt;&amp;&gt;">"'&lt;&amp;&gt;</marker>`},
	} {
		doc := etree.NewDocument()
		marker := doc.CreateElement("marker")
		marker.CreateAttr("label", c.attribute)
		marker.SetText(c.text)
		marker.SetTail(c.tail)
		before, err := doc.WriteToString()
		if err != nil {
			t.Fatal(err)
		}
		got, err := referencePreviewSerializeElement(marker)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Fatalf("lxml reference spelling: got %q, want %q", got, c.want)
		}
		after, err := doc.WriteToString()
		if err != nil || after != before {
			t.Fatal("serialization changed source", err)
		}
	}
}

// UNRUN token-boundary guard: references in literal data are not markup, and
// comments/PIs must not be rewritten by the value escaper.
func TestReferencePreviewElementEscapeTokenBoundaries(t *testing.T) {
	doc := etree.NewDocument()
	if err := doc.ReadFromString(`<root label="&amp;#xD; é">&amp;#13; é<!--&#xD;--><?keep &#xD;?><child/></root>`); err != nil {
		t.Fatal(err)
	}
	before, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	got, err := referencePreviewSerializeElement(doc.Root())
	if err != nil {
		t.Fatal(err)
	}
	want := `<root label="&amp;#xD; &#233;">&amp;#13; &#233;<!--&#xD;--><?keep &#xD;?><child/></root>`
	if string(got) != want {
		t.Fatalf("token escaping: got %q, want %q", got, want)
	}
	after, err := doc.WriteToString()
	if err != nil || after != before {
		t.Fatal("serialization changed source", err)
	}
}

// UNRUN retained-libxml2 token contracts. These exercise ASCII serialization of
// already acquired nodes, not lxml's default parser treatment of CDATA. Neither
// invalid comment/PI construction nor original parser acquisition is certified.
func TestReferencePreviewElementNonValueTokens(t *testing.T) {
	for _, c := range []struct{ xml, want string }{
		{`<root><!--<&amp; "quoted"--><?keep <&amp; "quoted"?></root>`, `<root><!--<&amp; "quoted"--><?keep <&amp; "quoted"?></root>`},
		{`<root><?empty?></root>`, `<root><?empty?></root>`},
		{`<root><![CDATA[<&amp; "quoted"]]><child label="&amp;"/>tail&amp;</root>`, `<root><![CDATA[<&amp; "quoted"]]><child label="&amp;"/>tail&amp;</root>`},
	} {
		doc := etree.NewDocument()
		doc.ReadSettings.PreserveCData = true
		if err := doc.ReadFromString(c.xml); err != nil {
			t.Fatal(err)
		}
		before, err := doc.WriteToString()
		if err != nil {
			t.Fatal(err)
		}
		got, err := referencePreviewSerializeElement(doc.Root())
		if err != nil || string(got) != c.want {
			t.Fatalf("non-value tokens: got %q, want %q, error %v", got, c.want, err)
		}
		after, err := doc.WriteToString()
		if err != nil || after != before {
			t.Fatal("serialization changed source", err)
		}
	}
	for _, c := range []struct{ data, want string }{
		{"", `<root><![CDATA[]]></root>`},
		{"]]>", `<root><![CDATA[]]]]><![CDATA[>]]></root>`},
		{"a]]>b]]>c", `<root><![CDATA[a]]]]><![CDATA[>b]]]]><![CDATA[>c]]></root>`},
	} {
		doc := etree.NewDocument()
		doc.ReadSettings.PreserveCData = true
		root := doc.CreateElement("root")
		data := root.CreateCData(c.data)
		got, err := referencePreviewSerializeElement(root)
		if err != nil || string(got) != c.want {
			t.Fatalf("CDATA splitting: got %q, want %q, error %v", got, c.want, err)
		}
		if data.Data != c.data || !data.IsCData() || len(root.Child) != 1 {
			t.Fatal("serialization changed source CDATA")
		}
	}
}

// UNRUN output-encoder contracts from retained libxml2 encoding.c. Numeric
// references in non-value tokens are literal bytes, not parsed character values.
// These programmatic/etree-acquired nodes do not certify lxml parser acquisition.
func TestReferencePreviewElementASCIIOutputEncoder(t *testing.T) {
	for _, c := range []struct{ xml, want string }{
		{`<root><!--é &amp; &#233;--></root>`, `<root><!--&#233; &amp; &#233;--></root>`},
		{`<root><?keep é &amp; &#233;?></root>`, `<root><?keep &#233; &amp; &#233;?></root>`},
		{`<root><![CDATA[é &amp; &#233;]]></root>`, `<root><![CDATA[&#233; &amp; &#233;]]></root>`},
		{`<root><!--😀--><?keep 😀?><![CDATA[😀]]></root>`, `<root><!--&#128512;--><?keep &#128512;?><![CDATA[&#128512;]]></root>`},
		// The encoder has no knowledge of XML names. Do not silently invent a
		// Unicode-name-preserving encoding when comparing original ASCII bytes.
		{`<é clé="é"/>`, `<&#233; cl&#233;="&#233;"/>`},
		{`<root><?é value?></root>`, `<root><?&#233; value?></root>`},
	} {
		doc := etree.NewDocument()
		doc.ReadSettings.PreserveCData = true
		if err := doc.ReadFromString(c.xml); err != nil {
			t.Fatal(err)
		}
		before, err := doc.WriteToString()
		if err != nil {
			t.Fatal(err)
		}
		got, err := referencePreviewSerializeElement(doc.Root())
		if err != nil || string(got) != c.want {
			t.Fatalf("ASCII output: got %q, want %q, error %v", got, c.want, err)
		}
		after, err := doc.WriteToString()
		if err != nil || after != before {
			t.Fatal("ASCII conversion changed source", err)
		}
	}
}
