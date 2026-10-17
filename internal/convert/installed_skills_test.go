// Installed-example tests execute the actual installed skill examples, using only the
// compiled CLI, a shell and (for Office rendering/calculation) LibreOffice.
package convert

import (
	"bytes"
	"context"
	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"github.com/xuri/excelize/v2"
)

// readSkillText mirrors Path.read_text's universal-newline handling before
// fence discovery. The regexp itself still requires literal LF delimiters.
func readSkillText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n"), nil
}

func shellBlocks(t *testing.T, text string) []string {
	t.Helper()
	// The original helper discovers complete fenced blocks with a non-greedy
	// regexp. Unclosed blocks are ignored; text inside a block is not reparsed.
	matches := regexp.MustCompile("(?s)```sh\\n(.*?)\\n```").FindAllStringSubmatch(text, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		blocks = append(blocks, match[1])
	}
	return blocks
}

func TestInstalledSkillExamples(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal("POSIX shell unavailable; installed examples cannot execute:", err)
	}
	// Like Path(__file__).resolve(), find examples relative to this source,
	// not the process cwd (a compiled test may be launched elsewhere).
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate installed-example test source")
	}
	root, err := installedExampleSourceRoot(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "paperworks"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(bin, name), "./cmd/paperworks")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	// All four original example families share one fresh working directory.
	work := filepath.Join(t.TempDir(), "examples")
	if err := createExampleOutput(work); err != nil {
		t.Fatal(err)
	}
	// The original helper executes every family before inspecting any output.
	for _, key := range []string{"pdf", "word", "spreadsheets", "presentations"} {
		if !t.Run("execute/"+key, func(t *testing.T) {
			source := filepath.Join(root, "testdata", "document-examples", "document_"+key)
			installed := filepath.Join(t.TempDir(), "installed skill")
			if err := os.CopyFS(installed, os.DirFS(source)); err != nil {
				t.Fatal(err)
			}
			data, err := readSkillText(filepath.Join(installed, "commands.txt"))
			if err != nil {
				t.Fatal(err)
			}
			blocks := shellBlocks(t, data)
			if !t.Run("runnable-examples", func(t *testing.T) {
				if len(blocks) == 0 {
					t.Fatal("No runnable example for " + key)
				}
			}) {
				t.FailNow()
			}
			for i, block := range blocks {
				out, err := runInstalledExample(ctx, shell, work, key, i, block,
					append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "SKILL_DIR="+installed), 60*time.Second)
				if err != nil {
					t.Fatalf("block %d: %v\n%s", i, err, out)
				}
			}
		}) {
			t.FailNow()
		}
	}
	// Verify original example outputs before the runtime render loop.
	for _, key := range []string{"pdf", "word", "spreadsheets", "presentations"} {
		if !t.Run(key, func(t *testing.T) {

			switch key {
			case "pdf":
				// Original standalone skill line 25; independent page-tree acquisition.
				runExampleCheck(t, "original-page-count", func(t *testing.T) {
					data, err := os.ReadFile(filepath.Join(work, "report.pdf"))
					if err != nil {
						t.Fatal(err)
					}
					count, err := pdfcpuapi.PageCount(ctx, bytes.NewReader(data), nil)
					if err != nil || count != 1 {
						t.Fatalf("pages=%d, want 1: %v", count, err)
					}
				})
				runExampleCheck(t, "original-table-row", func(t *testing.T) {
					data, err := os.ReadFile(filepath.Join(work, "report.pdf"))
					if err != nil {
						t.Fatal(err)
					}
					referencePDFAssertOriginalSkillTable(t, data)
				})
			case "presentations":
				// Preserve the original shipped-example contract, including its
				// first-slide selection. A larger replacement deck is not equivalent.
				p, e := opc.Open(filepath.Join(work, "report.pptx"))
				if e != nil {
					t.Fatal(e)
				}
				slides := referenceSkillSlides(t, p)
				if len(slides) == 0 {
					t.Fatalf("slides %v: %v", slides, e)
				}
				runExampleCheck(t, "original-slide-count", func(t *testing.T) {
					if len(slides) != 1 {
						t.Fatalf("slides = %d, want 1", len(slides))
					}
				})
				d, e := p.XML(slides[0])
				if e != nil {
					t.Fatal(e)
				}
				const c = "http://schemas.openxmlformats.org/drawingml/2006/chart"
				runExampleCheck(t, "original-chart-values", func(t *testing.T) {
					charts := referenceSkillFrames(d.Root(), c)
					if len(charts) == 0 {
						t.Fatal("first slide has no chart")
					}
					id := office.AttrV(charts[0], "http://schemas.openxmlformats.org/officeDocument/2006/relationships", "id")
					rels, err := p.Rels(slides[0])
					if err != nil {
						t.Fatal(err)
					}
					var part string
					for _, r := range rels {
						if r.ID == id {
							part = opc.Resolve(slides[0], r.Target)
						}
					}
					chart, err := p.XML(part)
					if err != nil {
						t.Fatal(err)
					}
					series := office.All(chart.Root(), c, "ser")
					if len(series) == 0 {
						t.Fatal("no series")
					}
					value := office.Child(series[0], c, "val")
					if value == nil {
						t.Fatal("no values")
					}
					points, err := referenceSkillSeriesValues(value)
					if err != nil {
						t.Fatal(err)
					}
					if len(points) != 2 {
						t.Fatalf("values count %d", len(points))
					}
					for i, want := range []float64{120, 80} {
						if points[i] == nil || *points[i] != want {
							t.Fatalf("point %d: %v, want %v", i, points[i], want)
						}
					}
				})
				runExampleCheck(t, "original-table", func(t *testing.T) {
					if len(referenceSkillFrames(d.Root(), "http://schemas.openxmlformats.org/drawingml/2006/table")) == 0 {
						t.Error("first slide has no table")
					}
				})
				runExampleCheck(t, "original-notes", func(t *testing.T) {
					text := referenceSkillNotes(t, p, slides[0])
					if !strings.Contains(text, "Total: $200.") {
						t.Fatalf("notes missing original total: %q", text)
					}
				})

			case "word":
				runExampleCheck(t, "original-table-and-title", func(t *testing.T) {
					p, e := opc.Open(filepath.Join(work, "report.docx"))
					if e != nil {
						t.Fatal(e)
					}
					d, e := p.XML("word/document.xml")
					if e != nil {
						t.Fatal(e)
					}
					const w = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
					body := office.Child(d.Root(), w, "body")
					tables := office.Children(body, w, "tbl")
					if len(tables) == 0 {
						t.Fatal("missing table")
					}
					rows := office.Children(tables[0], w, "tr")
					if len(rows) < 4 {
						t.Fatal("missing total row")
					}
					cells := office.Children(rows[3], w, "tc")
					if len(cells) < 2 {
						t.Fatal("missing amount cell")
					}
					text := referenceSkillCellText(cells[1])
					if text != "$200" {
						t.Fatalf("total %q", text)
					}
					paragraphs := office.Children(body, w, "p")
					if len(paragraphs) == 0 {
						t.Fatal("missing title")
					}
					props := office.Child(paragraphs[0], w, "pPr")
					styleID := office.AttrV(office.Child(props, w, "pStyle"), w, "val")
					styles, err := p.XML("word/styles.xml")
					if err != nil {
						t.Fatal(err)
					}
					name := referenceSkillStyleName(styles.Root(), styleID)
					if name != "Title" {
						t.Fatalf("first paragraph style name %q, want Title", name)
					}

				})
			case "spreadsheets":
				runExampleCheck(t, "original-workbook", func(t *testing.T) {
					f, e := excelize.OpenFile(filepath.Join(work, "report.xlsx"))
					if e != nil {
						t.Fatal(e)
					}
					defer f.Close()
					sheets := f.GetSheetList()
					if len(sheets) == 0 {
						t.Fatal("missing worksheet")
					}
					sheet := sheets[f.GetActiveSheetIndex()]
					formula, e := f.GetCellFormula(sheet, "D4")
					if e != nil || formula != "SUM(D2:D3)" {
						t.Fatalf("D4 formula %q: %v", formula, e)
					}

					data, e := os.ReadFile(filepath.Join(work, "report.xlsx"))
					if e != nil {
						t.Fatal(e)
					}
					parts, e := testsupport.ActiveWorksheetCharts(data)
					if e != nil || len(parts) != 1 {
						t.Fatalf("active worksheet charts %v: %v", parts, e)
					}
					validations, e := f.GetDataValidations(sheet)
					if e != nil || len(validations) != 1 {
						t.Fatalf("validations %v: %v", validations, e)
					}
					formats, e := f.GetConditionalFormats(sheet)
					if e != nil || len(formats) != 1 {
						t.Fatalf("conditional formatting %v: %v", formats, e)
					}
				})

			}
		}) {
			t.FailNow()
		}
	}
	// test_document_runtime calls examples() to completion before rendering.
	for _, key := range []string{"pdf", "word", "spreadsheets", "presentations"} {
		if !t.Run("runtime/"+key, func(t *testing.T) {
			// Original runtime lines 74-83: render all four actual shipped
			// examples, recalculating the spreadsheet first.
			t.Run("runtime-original-example", func(t *testing.T) {
				extension := map[string]string{"pdf": "pdf", "word": "docx", "spreadsheets": "xlsx", "presentations": "pptx"}[key]
				source := filepath.Join(work, "report."+extension)
				if extension == "xlsx" {
					result, e := Convert(ctx, source, filepath.Join(t.TempDir(), "calculated"), Options{Recalculate: true})
					if e != nil {
						t.Fatal(e)
					}
					source = result["output"].(string)
					f, e := excelize.OpenFile(source)
					if e != nil {
						t.Fatal(e)
					}
					defer f.Close()
					sheet := f.GetSheetName(f.GetActiveSheetIndex())
					runExampleCheck(t, "total", func(t *testing.T) {
						value, e := referenceSkillNumericCell(f, sheet, "D4")
						if e != nil || value != 200 {
							t.Fatalf("saved total %v: %v", value, e)
						}
					})
					runExampleCheck(t, "chart", func(t *testing.T) {
						data, e := os.ReadFile(source)
						if e != nil {
							t.Fatal(e)
						}
						parts, e := testsupport.ActiveWorksheetCharts(data)
						if e != nil {
							t.Fatal(e)
						}
						if len(parts) != 1 {
							t.Fatalf("charts %d, want 1", len(parts))
						}
					})
				}
				runExampleCheck(t, "render", func(t *testing.T) {
					images, e := RenderFile(ctx, source, filepath.Join(t.TempDir(), "render"), "", "", pdf.RenderOptions{})
					if e != nil || len(images) == 0 {
						t.Fatalf("render %v: %v", images, e)
					}
				})
			})
		}) {
			t.FailNow()
		}
	}
	// Additional native examples and later runtime checks cannot interrupt
	// the original execution/verification/render sequence above.
	for _, key := range []string{"pdf", "word", "spreadsheets", "presentations"} {
		if !t.Run("supplemental/"+key, func(t *testing.T) {
			switch key {
			case "pdf":
				// Supplemental text checks follow the complete original table
				// assertion and the runtime rendering phase above.
				text, err := os.ReadFile(filepath.Join(work, "flowing.txt"))
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"Revenue report", "Subscriptions", "$120", "Services", "$80", "Total", "$200"} {
					if !strings.Contains(string(text), want) {
						t.Fatalf("flowing example missing %q", want)
					}
				}
				figureText, err := os.ReadFile(filepath.Join(work, "figure.txt"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(figureText), "Figure 1.") || !strings.Contains(string(figureText), "Revenue equals price") {
					t.Fatal("missing figure caption")
				}
			case "presentations":

				t.Run("runtime-chart-edit", func(t *testing.T) {
					referenceRuntimeChartEdit(t, ctx, filepath.Join(work, "report.pptx"))
				})
			case "word":
				p, err := opc.Open(filepath.Join(work, "filled.docx"))
				if err != nil {
					t.Fatal(err)
				}
				var content string
				for _, part := range p.Parts {
					if part.Name == "word/document.xml" {
						content = string(part.Data)
					}
				}
				if !strings.Contains(content, "Example customer") || strings.Contains(content, "{{NAME}}") {
					t.Fatal("control value not replaced")
				}
			case "spreadsheets":
				files, err := filepath.Glob(filepath.Join(work, "calculated", "*.xlsx"))
				if err != nil || len(files) != 1 {
					t.Fatalf("calculated workbook: %v %v", files, err)
				}
				f, err := excelize.OpenFile(files[0])
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				value, err := f.GetCellValue("Revenue", "D4")
				if err != nil || !strings.Contains(value, "280") {
					t.Fatalf("recalculated total: %q %v", value, err)
				}
				formula, err := f.GetCellFormula("Revenue", "D4")
				if err != nil || formula != "SUM(D2:D3)" {
					t.Fatalf("formula lost: %q %v", formula, err)
				}
				for _, stage := range []struct {
					dir    string
					values []string
				}{
					{"sensitivity-calculated", []string{"102", "103", "202", "203"}},
					{"sensitivity-recalculated", []string{"202", "203", "402", "403"}},
				} {
					paths, e := filepath.Glob(filepath.Join(work, stage.dir, "*.xlsx"))
					if e != nil || len(paths) != 1 {
						t.Fatalf("missing sensitivity output: %v %v", paths, e)
					}
					book, e := excelize.OpenFile(paths[0])
					if e != nil {
						t.Fatal(e)
					}
					for i, cell := range []string{"E3", "F3", "E4", "F4"} {
						got, e := book.GetCellValue("Model", cell)
						if e != nil || got != stage.values[i] {
							t.Errorf("%s %s: %q want %q (%v)", stage.dir, cell, got, stage.values[i], e)
						}
					}
					book.Close()
				}
			}
		}) {
			t.FailNow()
		}
	}
}
