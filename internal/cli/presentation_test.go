package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReferenceNativePresentationDispatch(t *testing.T) {
	for _, c := range []struct{ op, file, key string }{{"slide-comments", "reference-comments.pptx", "comments"}, {"extended-charts", "reference-treemap.pptx", "charts"}} {
		r := runCLI(t, c.op, filepath.Join("../../testdata/fixtures", c.file))
		if len(r[c.key].([]any)) == 0 {
			t.Fatal(r)
		}
	}
}

func TestReferenceCategoricalHistogramResizeLabels(t *testing.T) {
	source := "../../testdata/fixtures/reference-categorical-histogram.pptx"
	p, err := opc.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := office.VerifyChartData(p, "")
	if err != nil || len(reports) == 0 {
		t.Fatal(reports, err)
	}
	raw, err := json.Marshal(reports[0])
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	part := report["chart"].(string)
	// Every failed edit leaves the output directory empty, so output() keeps
	// selecting 0.pptx, including the final successful edit.
	root := t.TempDir()
	specDir := t.TempDir()
	for _, c := range []struct {
		name   string
		set    bool
		labels any
	}{{"omitted", false, nil}, {"short", true, []string{"Only one"}}, {"numeric", true, []int{1, 2}}, {"null", true, nil}} {
		out := testsupport.PresentationOutput(t, root)
		s := map[string]any{"action": "replace-data", "chart_part": part, "values": []int{2, 4}}
		if c.set {
			s["categories"] = c.labels
		}
		spec := writeSpec(t, specDir, c.name+".json", s)
		var stdout, stderr bytes.Buffer
		code := Main([]string{"extended-charts", source, "--output", out, "--requirements", spec}, &stdout, &stderr)
		if code != ExitFailure || !strings.Contains(stderr.String(), "categories") {
			t.Fatalf("want categories failure: %d %s", code, stderr.String())
		}
		if strings.Contains(stderr.String(), "invalid JSON") {
			t.Fatalf("%s: rejection must occur in chart validation: %s", c.name, stderr.String())
		}
		wantDiagnostic := "Histogram categories must match values"
		if c.name == "null" {
			wantDiagnostic = "Histogram categories must be a list matching values"
		}
		if !strings.Contains(stderr.String(), wantDiagnostic) {
			t.Fatalf("%s: wrong chart validation failure: %s", c.name, stderr.String())
		}
		if _, e := os.Stat(out); !os.IsNotExist(e) {
			t.Fatal("published invalid output", e)
		}
	}
	out := testsupport.PresentationOutput(t, root)
	spec := writeSpec(t, specDir, "valid.json", map[string]any{"action": "replace-data", "chart_part": part, "values": []int{2, 4}, "categories": []string{"X", "Y"}})
	result := runCLI(t, "extended-charts", source, "--output", out, "--requirements", spec)
	dimensions := result["charts"].([]any)[0].(map[string]any)["series"].([]any)[0].(map[string]any)["dimensions"]
	if !reflect.DeepEqual(dimensions, map[string]any{"cat": []any{"X", "Y"}, "val": []any{float64(2), float64(4)}}) {
		t.Fatal(dimensions)
	}
}

func TestReferenceExplicitHistogramBinsPersistAndValidate(t *testing.T) {
	source := "../../testdata/fixtures/reference-histogram.pptx"
	dir := t.TempDir()
	specDir := t.TempDir()
	p, err := opc.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := office.VerifyChartData(p, "")
	if err != nil || len(reports) == 0 {
		t.Fatal(reports, err)
	}
	raw, err := json.Marshal(reports[0])
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	part := report["chart"].(string)
	out := testsupport.PresentationOutput(t, dir)
	spec := writeSpec(t, specDir, "bins.json", map[string]any{"action": "replace-data", "chart_part": part, "values": []int{1, 2, 7}, "bin_size": 5, "interval_closed": "r"})
	runCLI(t, "extended-charts", source, "--output", out, "--requirements", spec)
	assertSize := func(path string) {
		t.Helper()
		p, e := opc.Open(path)
		if e != nil {
			t.Fatal(e)
		}
		d, e := p.XML(part)
		if e != nil {
			t.Fatal(e)
		}
		// ElementTree .//binSize selects the first descendant, excluding root.
		var first *etree.Element
		for _, child := range d.Root().ChildElements() {
			nodes := office.All(child, office.NSCX, "binSize")
			if len(nodes) > 0 {
				first = nodes[0]
				break
			}
		}
		if first == nil || first.Text() != "5" {
			t.Fatal("binSize was not retained")
		}
	}
	assertSize(out)
	next := testsupport.PresentationOutput(t, dir)
	spec = writeSpec(t, specDir, "next.json", map[string]any{"action": "replace-data", "chart_part": part, "values": []int{1, 2, 8}})
	runCLI(t, "extended-charts", out, "--output", next, "--requirements", spec)
	assertSize(next)
	for _, c := range []struct {
		name       string
		extra      map[string]any
		diagnostic string
	}{{"zero", map[string]any{"bin_size": 0}, "bin_size"}, {"fractional_count", map[string]any{"bin_count": 1.5}, "bin_count"}, {"size_and_count", map[string]any{"bin_size": 2, "bin_count": 3}, "bin_size or bin_count"}, {"bad_interval", map[string]any{"interval_closed": "bad"}, "interval_closed"}} {
		// Failed edits leave the directory at two outputs, so Reference output()
		// chooses this same unused destination on every iteration.
		bad := testsupport.PresentationOutput(t, dir)
		s := map[string]any{"action": "replace-data", "chart_part": part, "values": []int{1, 2, 7}}
		for k, v := range c.extra {
			s[k] = v
		}
		spec := writeSpec(t, specDir, c.name+".json", s)
		var stdout, stderr bytes.Buffer
		if code := Main([]string{"extended-charts", source, "--output", bad, "--requirements", spec}, &stdout, &stderr); code != ExitFailure {
			t.Fatal("want chart-validation failure", c.extra, code, stderr.String())
		}
		if strings.Contains(stderr.String(), "invalid JSON") {
			t.Fatal("JSON decoding cannot satisfy the original chart validation", stderr.String())
		}
		if !strings.Contains(stderr.String(), c.diagnostic) {
			t.Fatal("unrelated rejection", c.name, stderr.String())
		}
		if _, e := os.Stat(bad); !os.IsNotExist(e) {
			t.Fatal("published invalid output", e)
		}
	}
}

func TestReferenceNewChartInvalidDataNoOutput(t *testing.T) {
	dir := t.TempDir()
	source := referenceNegativeFunnelFixture(t, "../../testdata/fixtures/reference-funnel.pptx", testsupport.PresentationOutput(t, dir))
	out := testsupport.PresentationOutput(t, dir)
	var e error
	if _, e = office.OfficePreviewCopy(source, out); e == nil || !strings.Contains(e.Error(), "nonnegative") {
		t.Fatal("want nonnegative rejection", e)
	}
	if _, e = os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("published invalid preview", e)
	}
	source = "../../testdata/fixtures/reference-waterfall.pptx"
	before, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	r := runCLI(t, "extended-charts", source)
	part := r["charts"].([]any)[0].(map[string]any)["chart"]
	spec := writeSpec(t, t.TempDir(), "replace.json", map[string]any{"action": "replace-data", "chart_part": part, "values": []int{1, 2, 3}})
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"extended-charts", source, "--output", out, "--requirements", spec}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "data replacement is not supported") {
		t.Fatalf("want unsupported replacement: %d %s", code, stderr.String())
	}
	if _, e = os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("published unsupported replacement", e)
	}
	after, e := os.ReadFile(source)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", e)
	}
}

// Reproduce PresentationNative.mutate and its negative callback: dictionary
// insertion order is the first occurrence; ZipFile.read(name) selects the last.
func referenceNegativeFunnelFixture(t *testing.T, original, output string) string {
	t.Helper()
	z, err := zip.OpenReader(original)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var names []string
	parts := map[string][]byte{}
	for _, member := range z.File {
		if _, exists := parts[member.Name]; !exists {
			names = append(names, member.Name)
		}
		r, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		closeErr := r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		parts[member.Name] = data
	}
	part := ""
	for _, name := range names {
		if strings.Contains(name, "extendedCharts/") && strings.HasSuffix(name, ".xml") {
			part = name
			break
		}
	}
	if part == "" {
		t.Fatal("missing original extended chart member")
	}
	d := etree.NewDocument()
	if err := d.ReadFromBytes(parts[part]); err != nil {
		t.Fatal(err)
	}
	// lxml .//numDim/lvl/pt: descendants for numDim, direct children thereafter,
	// selecting the first complete path, not merely the first numDim.
	var point *etree.Element
	var visit func(*etree.Element)
	visit = func(parent *etree.Element) {
		for _, child := range parent.ChildElements() {
			if office.Is(child, office.NSCX, "numDim") {
				for _, level := range child.ChildElements() {
					if !office.Is(level, office.NSCX, "lvl") {
						continue
					}
					for _, candidate := range level.ChildElements() {
						if office.Is(candidate, office.NSCX, "pt") {
							point = candidate
							return
						}
					}
				}
			}
			visit(child)
			if point != nil {
				return
			}
		}
	}
	visit(d.Root())
	if point == nil {
		t.Fatal("missing first numeric point")
	}
	point.SetText("-1")
	parts[part], err = d.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(parts[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return output
}

// Reference spec.get distinguishes omitted categories from explicit None. The
// typed API's nil slice remains omission; JSON must retain presence separately.
func TestCategoricalHistogramNullVersusOmitted(t *testing.T) {
	source := "../../testdata/fixtures/reference-categorical-histogram.pptx"
	pkg, err := opc.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := office.VerifyChartData(pkg, "")
	if err != nil || len(reports) != 1 {
		t.Fatal(reports, err)
	}
	raw, err := json.Marshal(reports[0])
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	part := report["chart"].(string)
	for _, explicitNull := range []bool{false, true} {
		name := "omitted"
		if explicitNull {
			name = "null"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			spec := map[string]any{"action": "replace-data", "chart_part": part, "values": []int{2, 4, 6}}
			if explicitNull {
				spec["categories"] = nil
			}
			request := writeSpec(t, dir, "spec.json", spec)
			output := filepath.Join(dir, "output.pptx")
			var stdout, stderr bytes.Buffer
			code := Main([]string{"extended-charts", source, "--output", output, "--requirements", request}, &stdout, &stderr)
			if explicitNull {
				if code != ExitFailure || !strings.Contains(stderr.String(), "categories must be a list") {
					t.Fatalf("null: %d %s", code, stderr.String())
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatalf("null published output: %v", err)
				}
			} else if code != ExitOK {
				t.Fatalf("omission: %d %s", code, stderr.String())
			}
		})
	}
	for _, label := range []any{nil, ""} {
		t.Run(fmt.Sprintf("label-%v", label), func(t *testing.T) {
			dir := t.TempDir()
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			request := writeSpec(t, dir, "spec.json", map[string]any{"action": "replace-data", "chart_part": part, "values": []int{2, 4, 6}, "categories": []any{"A", label, "C"}})
			output := filepath.Join(dir, "output.pptx")
			var stdout, stderr bytes.Buffer
			code := Main([]string{"extended-charts", source, "--output", output, "--requirements", request}, &stdout, &stderr)
			if label == nil {
				if code != ExitFailure || !strings.Contains(stderr.String(), "categories") {
					t.Fatalf("null label: %d %s", code, stderr.String())
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatalf("published output: %v", err)
				}
			} else if code != ExitOK {
				t.Fatalf("empty string: %d %s", code, stderr.String())
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
	t.Run("typed-nil-is-omitted", func(t *testing.T) {
		p, err := opc.Open(source)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = office.ExtendedCharts(p, office.ExtendedChartSpec{Action: "replace-data", Part: part, Values: []float64{2, 4, 6}}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCategoricalAddInvalidLabelsNoOutput(t *testing.T) {
	source := "../../testdata/fixtures/reference-categorical-histogram.pptx"
	for _, labels := range []any{[]any{1, 2}, []any{nil, "Y"}} {
		dir := t.TempDir()
		spec := writeSpec(t, dir, "spec.json", map[string]any{"action": "add", "type": "histogram", "slide": 1, "position_emu": []int{0, 0, 1000000, 1000000}, "values": []int{2, 4}, "categories": labels})
		out := filepath.Join(dir, "out.pptx")
		var stdout, stderr bytes.Buffer
		code := Main([]string{"extended-charts", source, "--output", out, "--requirements", spec}, &stdout, &stderr)
		if code != ExitFailure || !strings.Contains(stderr.String(), "Histogram categories must match values") || strings.Contains(stderr.String(), "invalid JSON") {
			t.Fatalf("%d %s", code, stderr.String())
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("partial output: %v", err)
		}
	}
}
