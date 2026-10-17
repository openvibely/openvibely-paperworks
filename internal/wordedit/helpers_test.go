package wordedit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"`

// mk writes a DOCX whose body is body; extra parts may override any part.
func mk(t *testing.T, name, body string, extra map[string]string) string {
	t.Helper()
	p, err := opc.Open(referenceOriginalWordFixture(t, "blank-template"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := p.XML("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	originalBody := office.Child(doc.Root(), nsW, "body")
	section := office.Child(originalBody, nsW, "sectPr")
	for _, child := range originalBody.ChildElements() {
		if child != section {
			originalBody.RemoveChild(child)
		}
	}
	fragment := etree.NewDocument()
	if err := fragment.ReadFromString(`<w:body ` + wNS + `>` + body + `</w:body>`); err != nil {
		t.Fatal(err)
	}
	for _, child := range fragment.Root().ChildElements() {
		originalBody.InsertChildAt(section.Index(), child.Copy())
	}
	if err := p.SetXML("word/document.xml", doc); err != nil {
		t.Fatal(err)
	}
	for k, v := range extra {
		p.Set(k, []byte(v))
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name+".docx")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func base(t *testing.T) string {
	t.Helper()
	// Preserve the complete pinned WordEditing.setUp package, including the
	// template styles, theme, numbering, and original-source snapshot.
	return referenceOriginalWordFixture(t, "base")
}

// edit runs one operation and checks that the source is untouched.
func edit(t *testing.T, src string, spec map[string]any) (string, error) {
	t.Helper()
	if out, err, routed := referenceWordRoutedEdit(t, src, spec); routed {
		return out, err
	}
	return referenceNamedEdit(t, t.TempDir(), "out", src, spec)
}

// referenceNamedEdit retains the original root/name routing. An empty source uses
// the original setup document; a repeated name reaches Edit's overwrite guard.
func referenceNamedEdit(t *testing.T, root, name, src string, spec map[string]any) (string, error) {
	t.Helper()
	ensureReferenceOriginal(t)
	if src == "" {
		forReferenceOriginals(t, func(snapshot referenceOriginalSnapshot) {
			if src == "" {
				src = snapshot.path
			}
		})
	}
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	out := name + ".docx"
	// PurePosixPath removes empty and dot components, but retains parents.
	// Preserve its special exactly-two-leading-slashes root as well.
	prefix := ""
	if strings.HasPrefix(out, "/") {
		prefix = "/"
		if strings.HasPrefix(out, "//") && !strings.HasPrefix(out, "///") {
			prefix = "//"
		}
	}
	components := []string{}
	for _, component := range strings.Split(out, "/") {
		if component != "" && component != "." {
			components = append(components, component)
		}
	}
	out = prefix + strings.Join(components, "/")
	// pathlib's root / absolute_name preserves the absolute operand.
	if !filepath.IsAbs(out) {
		// Do not clean parent components: pathlib leaves their resolution to
		// the filesystem, including missing intermediate directories.
		out = root + string(os.PathSeparator) + out
	}
	existing, statErr := os.ReadFile(out)
	if statErr != nil && !os.IsNotExist(statErr) {
		t.Fatal(statErr)
	}
	_, err = Edit(src, out, spec, filepath.Dir(src))
	assertReferenceOriginals(t)
	after, readErr := os.ReadFile(src)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
	if err != nil {
		saved, readErr := os.ReadFile(out)
		if statErr == nil {
			if readErr != nil || !bytes.Equal(existing, saved) {
				t.Fatal("failed edit changed existing output", readErr)
			}
		} else if !os.IsNotExist(readErr) {
			t.Fatalf("failed edit published output: %v", err)
		}
	}
	return out, err
}

func mustEdit(t *testing.T, src string, spec map[string]any) string {
	t.Helper()
	out, err := edit(t, src, spec)
	if err != nil {
		t.Fatalf("%v: %v", spec["operation"], err)
	}
	return out
}

func wantErr(t *testing.T, src string, spec map[string]any, pattern string) {
	t.Helper()
	_, err := edit(t, src, spec)
	if err == nil || !regexp.MustCompile("(?i)"+pattern).MatchString(err.Error()) {
		t.Fatalf("%v: want error %q, got %v", spec["operation"], pattern, err)
	}
}

func part(t *testing.T, path, name string) string {
	t.Helper()
	p, err := opc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(p.Get(name))
}

func unescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
}

func paras(t *testing.T, path string) []string {
	return referenceStoryTexts(t, path, "word/document.xml", false)
}

func report(t *testing.T, path string) map[string]any {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := office.PackageReport(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("inspection changed source")
	}
	return asMap(t, r)
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func contains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("missing %q in %.400s", sub, s)
	}
}

func absent(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Fatalf("unexpected %q", sub)
	}
}

func withCore(author string) map[string]string {
	return map[string]string{"docProps/core.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title></dc:title><dc:creator>` + author + `</dc:creator></cp:coreProperties>`}
}

// UNRUN helper-contract correction. Reference WordEditing.edit checks the setUp
// original even when the operation receives a different or intermediate file.
// Keep originals for the test and its ancestor subtests, not just the input of
// the latest operation. Multiple base fixtures in a Go case are all protected.
var referenceOriginals sync.Map // test name -> []referenceOriginalSnapshot

type referenceOriginalSnapshot struct {
	path string
	data []byte
}

func registerReferenceOriginal(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []referenceOriginalSnapshot
	if previous, ok := referenceOriginals.Load(t.Name()); ok {
		snapshots = previous.([]referenceOriginalSnapshot)
	} else {
		t.Cleanup(func() { referenceOriginals.Delete(t.Name()) })
	}
	snapshot := referenceOriginalSnapshot{path, data}
	// Each fixture is checked before its own TempDir cleanup, including when
	// a test creates multiple base fixtures.
	t.Cleanup(func() { assertReferenceOriginal(t, snapshot) })
	snapshots = append(snapshots, snapshot)
	referenceOriginals.Store(t.Name(), snapshots)
}

func forReferenceOriginals(t *testing.T, visit func(referenceOriginalSnapshot)) bool {
	t.Helper()
	found := false
	for name := t.Name(); ; {
		if entry, ok := referenceOriginals.Load(name); ok {
			found = true
			for _, snapshot := range entry.([]referenceOriginalSnapshot) {
				visit(snapshot)
			}
		}
		i := strings.LastIndexByte(name, '/')
		if i < 0 {
			break
		}
		name = name[:i]
	}
	return found
}

func ensureReferenceOriginal(t *testing.T) {
	t.Helper()
	if !forReferenceOriginals(t, func(referenceOriginalSnapshot) {}) {
		// Alternate-source Reference cases still have an untouched setUp document.
		base(t)
	}
}

func assertReferenceOriginals(t *testing.T) {
	t.Helper()
	forReferenceOriginals(t, func(snapshot referenceOriginalSnapshot) { assertReferenceOriginal(t, snapshot) })
}

func assertReferenceOriginal(t *testing.T, snapshot referenceOriginalSnapshot) {
	t.Helper()
	after, err := os.ReadFile(snapshot.path)
	if err != nil {
		t.Errorf("original fixture %s: %v", snapshot.path, err)
		return
	}
	if !bytes.Equal(after, snapshot.data) {
		t.Errorf("original fixture changed: %s", snapshot.path)
	}
}
