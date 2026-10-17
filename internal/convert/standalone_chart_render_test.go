package convert

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// Original histogram cases retain separate geometry, text, image, and source
// assertions. Bounded reader comparisons do not establish general PDF equivalence.
func TestReferenceStandaloneRuntimeHistogramBins(t *testing.T) {
	referenceRuntimeOffice(t)
	for _, c := range []struct {
		name   string
		values []float64
		labels []string
		tails  bool
	}{
		{"automatic", []float64{7, 15, 30}, []string{"[7, 27]", "(27, 47]"}, false},
		{"tails", []float64{-2, 0, 2, 4, 6}, []string{"≤ 0", "(0, 2]", "(2, 4]", "> 4"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, e := opc.Open("../../testdata/fixtures/reference-categorical-histogram.pptx")
			if e != nil {
				t.Fatal(e)
			}
			reports, e := office.ExtendedCharts(p, office.ExtendedChartSpec{})
			if e != nil || len(reports) == 0 {
				t.Fatal(reports, e)
			}
			spec := office.ExtendedChartSpec{Action: "replace-data", Part: reports[0].Chart, Values: c.values}
			for i := range c.values {
				spec.Categories = append(spec.Categories, strconv.Itoa(i))
			}
			if c.tails {
				count := 4
				spec.BinCount = &count
				spec.Underflow = float64(0)
				spec.Overflow = float64(4)
			}
			if _, e = office.ExtendedCharts(p, spec); e != nil {
				t.Fatal(e)
			}
			dir := t.TempDir()
			src := filepath.Join(dir, "histogram-"+c.name+".pptx")
			if e = p.SaveNew(src); e != nil {
				t.Fatal(e)
			}
			before := contents(t, src)
			out := filepath.Join(dir, "render")
			images, e := RenderFile(context.Background(), src, out, "", "", pdf.RenderOptions{})
			if e != nil {
				t.Fatal(e)
			}
			t.Run("images", func(t *testing.T) {
				if len(images) != 1 {
					t.Errorf("images %d, want 1", len(images))
				}
			})
			pages := referenceRuntimePDFTexts(t, contents(t, filepath.Join(out, "histogram-"+c.name+".pdf")))
			if len(pages) == 0 {
				t.Fatal("no PDF pages")
			}
			t.Run("labels", func(t *testing.T) {
				for _, label := range c.labels {
					t.Run(label, func(t *testing.T) {
						if !strings.Contains(pages[0], label) {
							t.Errorf("missing %q in %q", label, pages[0])
						}
					})
				}
			})
			t.Run("supported", func(t *testing.T) {
				if strings.Contains(strings.ToLower(pages[0]), "unsupported") {
					t.Errorf("unsupported: %s", pages[0])
				}
			})
			t.Run("geometry", func(t *testing.T) {
				marks := referencePDFPaths(t, contents(t, filepath.Join(out, "histogram-"+c.name+".pdf")))
				if got := referencePDFCurveRectCount(marks); got < len(c.labels) {
					t.Errorf("curves+rects=%d, want >=%d", got, len(c.labels))
				}
			})
			t.Run("source", func(t *testing.T) {
				if !bytes.Equal(before, contents(t, src)) {
					t.Error("source changed")
				}
			})
			t.Run("extended", func(t *testing.T) {
				saved, e := opc.Open(src)
				if e != nil {
					t.Fatal(e)
				}
				rs, e := office.VerifyChartData(saved, "")
				if e != nil || len(rs) == 0 {
					t.Fatal(rs, e)
				}
				r, ok := rs[0].(*office.ChartExReport)
				if !ok || !r.Extended {
					t.Errorf("expected extended chart: %#v", rs[0])
				}
			})
		})
	}
}

func TestReferenceStandaloneRuntimeHistogramNumberFormats(t *testing.T) {
	referenceRuntimeOffice(t)
	for _, c := range []struct {
		name   string
		labels []string
	}{
		{"padding", []string{"0000", "0002", "0004", "0006", "0008", "0010"}},
		{"scientific", []string{"0.00E+00", "2.00E+00", "4.00E+00", "6.00E+00", "8.00E+00", "1.00E+01"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			stem := "reference-histogram-" + c.name
			src := filepath.Join("../../testdata/fixtures", stem+".pptx")
			before := contents(t, src)
			out := filepath.Join(t.TempDir(), "render")
			images, e := RenderFile(context.Background(), src, out, "", "", pdf.RenderOptions{})
			if e != nil {
				t.Fatal(e)
			}
			t.Run("images", func(t *testing.T) {
				if len(images) != 1 {
					t.Fatalf("images %d, want 1", len(images))
				}
				st, e := os.Stat(images[0])
				if e != nil {
					t.Fatal(e)
				}
				if st.Size() <= 0 {
					t.Error("empty rendered image")
				}
			})
			pages := referenceRuntimePDFTexts(t, contents(t, filepath.Join(out, stem+".pdf")))
			if len(pages) == 0 {
				t.Fatal("no PDF pages")
			}
			t.Run("labels", func(t *testing.T) {
				for _, label := range c.labels {
					t.Run(label, func(t *testing.T) {
						if !strings.Contains(pages[0], label) {
							t.Errorf("missing %q in %q", label, pages[0])
						}
					})
				}
			})
			t.Run("source", func(t *testing.T) {
				if !bytes.Equal(before, contents(t, src)) {
					t.Error("source changed")
				}
			})
		})
	}
}

// All five PPTX cases and all three XLSX cases from runtime lines 916-972.
// Geometry sites 926-947, 961-962, 968-971 are written and UNRUN.
func TestReferenceStandaloneRuntimeStandardChartContent(t *testing.T) {
	referenceRuntimeOffice(t)
	for _, ext := range []string{"pptx", "xlsx"} {
		t.Run(ext, func(t *testing.T) {
			kinds := []string{"waterfall", "funnel", "pie", "doughnut", "scatter"}
			if ext == "xlsx" {
				kinds = []string{"pie", "doughnut", "scatter"}
			}
			for _, kind := range kinds {
				t.Run(kind, func(t *testing.T) {
					stem := "reference-" + kind
					src := filepath.Join("../../testdata/fixtures", stem+"."+ext)
					before := contents(t, src)
					out := filepath.Join(t.TempDir(), "render")
					sheet, area := "", ""
					if ext == "xlsx" {
						sheet, area = "Chart", "D2:O23"
					}
					images, e := RenderFile(context.Background(), src, out, sheet, area, pdf.RenderOptions{})
					if e != nil {
						t.Fatal(e)
					}
					t.Run("images", func(t *testing.T) {
						if len(images) != 1 {
							t.Errorf("images %d, want 1", len(images))
						}
					})
					pages := referenceRuntimePDFTexts(t, contents(t, filepath.Join(out, stem+".pdf")))
					if len(pages) == 0 {
						t.Fatal("rendered PDF has no first page")
					}
					checkLabels := func() {
						if kind != "scatter" {
							t.Run("labels", func(t *testing.T) {
								for _, label := range []string{"A", "B", "C"} {
									t.Run(label, func(t *testing.T) {
										found := strings.Contains(pages[0], label)
										if ext == "xlsx" {
											found = false
											for _, word := range strings.Fields(pages[0]) {
												if word == label {
													found = true
												}
											}
										}
										if !found {
											t.Errorf("missing %q in %q", label, pages[0])
										}
									})
								}
							})
						}
					}
					// Reference checks PPTX labels before geometry, XLSX labels after it.
					if ext == "pptx" {
						checkLabels()
					}
					t.Run("geometry", func(t *testing.T) {
						paths := referencePDFPaths(t, contents(t, filepath.Join(out, stem+".pdf")))
						var marks []referencePDFPath
						for _, p := range paths {
							w, h := p.x1-p.x0, p.y1-p.y0
							curve := !p.line && !p.rect
							switch kind {
							case "pie", "doughnut":
								if curve && p.filled && w > 50 && h > 50 && (ext != "xlsx" || p.bezier) {
									marks = append(marks, p)
								}
							case "scatter":
								if curve && p.filled && w > 2 && w < 4 && h > 2 && h < 4 {
									marks = append(marks, p)
								}
							default:
								color := referencePDFPathColor(p, false)
								if len(color) != 3 {
									continue
								}
								r, g, b := color[0], color[1], color[2]
								if !p.line && p.filled && w > 50 && h > 20 && r < .2 && g > .3 && g < .5 && b > .45 && b < .6 {
									marks = append(marks, p)
								}
							}
						}
						t.Run("count", func(t *testing.T) {
							if len(marks) != 3 {
								t.Errorf("marks=%+v; want 3", marks)
							}
						})
						switch kind {
						case "pie", "doughnut":
							t.Run("colors", func(t *testing.T) {
								colors := map[string]bool{}
								for _, p := range marks {
									colors[fmt.Sprint(referencePDFPathColor(p, false))] = true
								}
								if len(colors) != 3 {
									t.Errorf("colors=%v; want 3", colors)
								}
							})
						case "scatter":
							t.Run("connections", func(t *testing.T) {
								for _, p := range paths {
									if p.line || p.rect || !p.stroked || p.x1-p.x0 <= 20 || p.y1-p.y0 <= 20 {
										continue
									}
									sameColor := ext == "pptx"
									for _, m := range marks {
										if fmt.Sprint(referencePDFPathColor(p, true)) == fmt.Sprint(referencePDFPathColor(m, false)) {
											sameColor = true
										}
									}
									if sameColor {
										t.Errorf("unexpected connection: %+v", p)
									}
								}
							})
						default:
							if kind == "waterfall" {
								sort.SliceStable(marks, func(i, j int) bool { return marks[i].x0 < marks[j].x0 })
							} else {
								sort.SliceStable(marks, func(i, j int) bool { return marks[i].y1 > marks[j].y1 })
							}
							for i, want := range []float64{2, .5} {
								if kind == "funnel" {
									want = []float64{.5, .25}[i]
								}
								t.Run(fmt.Sprintf("ratio-%d", i), func(t *testing.T) {
									if len(marks) != 3 {
										t.Fatalf("cannot inspect ratio: marks=%+v", marks)
									}
									got := (marks[i+1].y1 - marks[i+1].y0) / (marks[0].y1 - marks[0].y0)
									if kind == "funnel" {
										index := []int{0, 2}[i]
										got = (marks[index].x1 - marks[index].x0) / (marks[1].x1 - marks[1].x0)
									}
									if !(math.Abs(got-want) < .01) {
										t.Errorf("ratio=%g want %g within .01", got, want)
									}
								})
							}
						}
					})
					if ext == "xlsx" {
						checkLabels()
					}

					if ext == "xlsx" && kind != "scatter" {
						t.Run("no-series-label", func(t *testing.T) {
							if strings.Contains(pages[0], "Values") {
								t.Errorf("unexpected Values: %q", pages[0])
							}
						})
					}
					t.Run("source", func(t *testing.T) {
						if !bytes.Equal(before, contents(t, src)) {
							t.Error("source changed")
						}
					})
				})
			}
		})
	}
}
