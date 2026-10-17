package cli

import (
	"archive/zip"
	"bytes"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PresentationNative.test_bad_inputs_do_not_publish: the same unused output
// follows the failed comment operation into the non-finite chart operation.
func TestReferenceBadInputsDoNotPublish(t *testing.T) {
	dir := t.TempDir()
	source := "../../testdata/fixtures/reference-anchors.pptx"
	out := testsupport.PresentationOutput(t, dir)
	spec := writeSpec(t, t.TempDir(), "comment.json", map[string]any{"action": "add", "slide": 1, "shape_id": 2, "text_match": "absent", "author": "X", "text": "Y"})
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"slide-comments", source, "--requirements", spec, "--output", out}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "not found") {
		t.Fatal(code, stderr.String())
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("published invalid comment", err)
	}
	// JSON cannot carry Reference's float('nan'). Pass the typed value to the
	// chart implementation, then use the same save-on-success boundary as the
	// native command. A JSON syntax rejection cannot satisfy this assertion.
	chartEdit := func() error {
		if err := newFile(out, source); err != nil {
			return err
		}
		p, err := opc.Open(source)
		if err != nil {
			return err
		}
		if _, err = office.ExtendedCharts(p, office.ExtendedChartSpec{Action: "add", Values: []float64{math.NaN()}}); err != nil {
			return err
		}
		return p.SaveNew(out)
	}
	if err := chartEdit(); err == nil || !strings.Contains(err.Error(), "finite") {
		t.Fatal("wrong chart validation failure", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("published invalid chart", err)
	}
}

// Supplemental JSON-boundary checks are not the original typed NaN contract.
func TestNonfiniteJSONDoesNotPublish(t *testing.T) {
	for _, token := range []string{"NaN", "Infinity", "-Infinity"} {
		t.Run(token, func(t *testing.T) {
			dir := t.TempDir()
			spec := filepath.Join(dir, "spec.json")
			out := filepath.Join(dir, "new.pptx")
			if err := os.WriteFile(spec, []byte(`{"action":"add","values":[`+token+`]}`), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := Main([]string{"extended-charts", "../../testdata/fixtures/reference-anchors.pptx", "--requirements", spec, "--output", out}, &stdout, &stderr)
			if code == 0 || !strings.Contains(stderr.String(), "finite") {
				t.Fatal(code, stderr.String())
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("published invalid output", err)
			}
		})
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "quoted.json")
	if err := os.WriteFile(p, []byte(`{"text":"NaN Infinity -Infinity"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := readJSONFile(p, &result); err != nil {
		t.Fatal(err)
	}
}

// Preserve the absent input and sequential cases of the original validation probe.
// The diagnostic prevents an unrelated filesystem failure satisfying the contract.
func TestReferenceRendererResolutionValidation(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		args       []string
		diagnostic string
	}{
		{[]string{"--dpi", "0"}, "DPI must be between 36 and 600"},
		{[]string{"--dpi", "NaN"}, "DPI must be between 36 and 600"},
		{[]string{"--max-pixels", "10"}, "max_pixels must be between 100 and 10000"},
		{[]string{"--max-pixels", "0"}, "max_pixels must be between 100 and 10000"},
	} {
		var stdout, stderr bytes.Buffer
		command := append([]string{"render", filepath.Join(dir, "unused.pdf"), "--output", dir}, c.args...)
		if code := Main(command, &stdout, &stderr); code == 0 {
			t.Fatal("invalid resolution accepted", c.args)
		}
		if !strings.Contains(stderr.String(), c.diagnostic) {
			t.Fatal("wrong validation failure", c.args, stderr.String())
		}
	}
}
func TestReferenceXMLEntityFailsWithoutStdout(t *testing.T) {
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	w, e := z.Create("word/document.xml")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.Write([]byte(`<!DOCTYPE x [<!ENTITY x "bad">]><x>&x;</x>`)); e != nil {
		t.Fatal(e)
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	src := filepath.Join(t.TempDir(), "bad.docx")
	if e = os.WriteFile(src, archive.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := office.PackageReport(src); e == nil {
		t.Fatal("entity accepted")
	}
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"inspect", src}, &stdout, &stderr); code == 0 {
		t.Fatal("invalid input accepted")
	}
	if stdout.Len() != 0 {
		t.Fatal("unexpected stdout", stdout.String())
	}
}
