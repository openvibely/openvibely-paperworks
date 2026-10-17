package convert

// The independent table pipeline preserves the original structural assertion.
// Same-input evidence covers the original and native installed examples;
// unsupported PDF acquisition/metrics remain an explicit general equivalence gap.
import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"
	"time"
)

func referencePDFExtractTables(g referencePDFGeometry) [][][]*string {
	edges := referencePDFMergeTableEdges(referencePDFOrientedTableEdges(g), 3, 3, 3, 3, 3)
	cells := referencePDFTableCells(edges, 3, 3)
	var tables [][][]*string
	for _, group := range referencePDFGroupTableCells(cells) {
		tables = append(tables, referencePDFExtractRows(group, g.chars,
			func(c referencePDFDirectedChar) referencePDFCell { return c.char.box }, referencePDFDefaultCellText))
	}
	return tables
}

// Original test_skill_examples.py:26: first extracted table, final row, exact
// two-column equality. This executes the shipped PDF examples, not a synthetic
// report, XML table, substring search, or search for a matching row anywhere.
func TestReferenceSkillExampleExtractedTableRow(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(t.TempDir(), "installed skill")
	if err := os.CopyFS(installed, os.DirFS(filepath.Join(root, "testdata", "document-examples", "document_pdf"))); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(installed, "commands.txt"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```sh\\n(.*?)\\n```").FindAllSubmatch(source, -1)
	if len(blocks) == 0 {
		t.Fatal("No runnable example for pdf")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	bin, work := t.TempDir(), t.TempDir()
	executable := "paperworks"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(bin, executable), "./cmd/paperworks")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for i, block := range blocks {
		cmd := exec.CommandContext(ctx, "sh", "-eu", "-c", string(block[1]))
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "SKILL_DIR="+installed)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("PDF example %d: %v\n%s", i, err, out)
		}
	}
	data, err := os.ReadFile(filepath.Join(work, "report.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	referencePDFAssertOriginalSkillTable(t, data)
}

// referencePDFAssertOriginalSkillTable preserves test_skill_examples.py:26:
// first table, last row, exact cell count, values and nullability. It is called
// in the installed helper's verification phase as well as its direct regression.
func referencePDFAssertOriginalSkillTable(t *testing.T, data []byte) {
	t.Helper()
	tables := referencePDFExtractTables(referencePDFPageGeometry(t, data, 0, true))
	if len(tables) == 0 || len(tables[0]) == 0 {
		t.Fatal("first extracted table has no rows")
	}
	total, amount := "Total", "$200"
	want := []*string{&total, &amount}
	got := tables[0][len(tables[0])-1]
	if !reflect.DeepEqual(got, want) {
		var values []any
		for _, cell := range got {
			if cell == nil {
				values = append(values, nil)
			} else {
				values = append(values, *cell)
			}
		}
		t.Fatalf("first table final row=%v; want exactly [Total $200] (including column count and non-null cells)", values)
	}
}

// Integration contracts are UNRUN; they feed geometry/characters, not a PDF,
// and do not establish correctness of the PDFium acquisition adapter.
func TestReferencePDFTablePipelineContracts(t *testing.T) {
	line := func(x0, y0, x1, y1 float64) referencePDFPath {
		return referencePDFPath{line: true, points: [][2]float64{{x0, y0}, {x1, y1}}, x0: x0, y0: y0, x1: x1, y1: y1}
	}
	g := referencePDFGeometry{height: 100}
	for _, x := range []float64{0, 50, 100} {
		g.paths = append(g.paths, line(x, 60, x, 100))
	}
	for _, y := range []float64{60, 80, 100} {
		g.paths = append(g.paths, line(0, y, 100, y))
	}
	ch := func(text string, x, top float64, upright bool) referencePDFDirectedChar {
		return referencePDFDirectedChar{referencePDFTableChar{text, referencePDFCell{x, top, x + 5, top + 5}}, upright}
	}
	// Keep separate glyphs and their orientation through midpoint assignment.
	g.chars = []referencePDFDirectedChar{ch("A", 5, 5, true), ch("B", 55, 5, false), ch("C", 55, 11, false), ch("D", 5, 25, true)}
	a, bc, d, empty := "A", "BC", "D", ""
	want := [][][]*string{{{&a, &bc}, {&d, &empty}}}
	if got := referencePDFExtractTables(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("pipeline=%v, want %v", got, want)
	}
}
