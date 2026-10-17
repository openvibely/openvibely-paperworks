package convert

// Test-only vector inspection for the original pdfplumber assertions. It reads
// actual PDF paths, never raster pixels or OOXML shapes. UNRUN during porting.
import (
	"context"
	"math"
	"testing"

	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/structs"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

type referencePDFPath struct {
	// A straight two-point subpath is a line; all other nonempty subpaths
	// contribute to pdfplumber's combined curves+rects count.
	exactFill, exactStroke        []float64
	line                          bool
	linewidth                     float64
	points                        [][2]float64
	rect, bezier, filled, stroked bool
	x0, y0, x1, y1                float64
	fill                          structs.FPDF_COLOR
	stroke                        structs.FPDF_COLOR
}

type referencePDFWord struct {
	text                string
	x0, x1, top, bottom float64
}

type referencePDFGeometry struct {
	paths  []referencePDFPath
	words  []referencePDFWord
	chars  []referencePDFDirectedChar
	height float64
}

func referencePDFPaths(t *testing.T, data []byte) []referencePDFPath {
	return referencePDFPageGeometry(t, data, 0, false).paths
}

// Word boxes are a written adapter, not yet established as equivalent to
// PDFMiner LTChar metrics. Review loose-box metrics, grouping and page rotation
// before accepting any geometry assertion as passing evidence.
func referencePDFPageGeometry(t *testing.T, data []byte, pageIndex int, withWords bool) referencePDFGeometry {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	pool, err := webassembly.Init(webassembly.Config{
		MinIdle: 1, MaxIdle: 1, MaxTotal: 1, ReuseWorkers: true,
		RuntimeConfig: wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).WithMemoryLimitPages(8192),
	})
	must(err)
	defer pool.Close()
	inst, err := pool.GetInstanceWithContext(context.Background())
	must(err)
	defer inst.Close()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	must(err)
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	page, err := inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc.Document, Index: pageIndex})
	must(err)
	defer inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})
	ref := requests.Page{ByReference: &page.Page}
	var paths []referencePDFPath
	// Compose object and enclosing Form matrices before measuring page-space paths.
	type matrix = referencePDFMatrix
	// PDFium object coordinates do not include its display/page matrix.
	// PDFMiner uses MediaBox rather than PDFium's CropBox-based page size.
	media, err := inst.FPDFPage_GetMediaBox(&requests.FPDFPage_GetMediaBox{Page: ref})
	must(err)
	rotation, err := inst.FPDFPage_GetRotation(&requests.FPDFPage_GetRotation{Page: ref})
	must(err)
	pageMatrix, _, pageHeight := referencePDFMinerPageCoordinates(
		[4]float64{float64(media.Left), float64(media.Bottom), float64(media.Right), float64(media.Top)},
		int(rotation.PageRotation)*90)

	var visit func(references.FPDF_PAGEOBJECT, matrix)
	visit = func(obj references.FPDF_PAGEOBJECT, parent matrix) {
		response, err := inst.FPDFPageObj_GetMatrix(&requests.FPDFPageObj_GetMatrix{PageObject: obj})
		must(err)
		m := response.Matrix
		transform := referencePDFComposeMatrix(parent, matrix{float64(m.A), float64(m.B), float64(m.C), float64(m.D), float64(m.E), float64(m.F)})
		kind, err := inst.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: obj})
		must(err)
		if kind.Type == enums.FPDF_PAGEOBJ_FORM {
			n, err := inst.FPDFFormObj_CountObjects(&requests.FPDFFormObj_CountObjects{PageObject: obj})
			must(err)
			for i := 0; i < n.Count; i++ {
				child, err := inst.FPDFFormObj_GetObject(&requests.FPDFFormObj_GetObject{PageObject: obj, Index: uint64(i)})
				must(err)
				visit(child.PageObject, transform)
			}
			return
		}
		if kind.Type != enums.FPDF_PAGEOBJ_PATH {
			return
		}
		color, err := inst.FPDFPageObj_GetFillColor(&requests.FPDFPageObj_GetFillColor{PageObject: obj})
		must(err)
		stroke, err := inst.FPDFPageObj_GetStrokeColor(&requests.FPDFPageObj_GetStrokeColor{PageObject: obj})
		must(err)
		n, err := inst.FPDFPath_CountSegments(&requests.FPDFPath_CountSegments{PageObject: obj})
		must(err)
		paint, err := inst.FPDFPath_GetDrawMode(&requests.FPDFPath_GetDrawMode{PageObject: obj})
		must(err)
		width, err := inst.FPDFPageObj_GetStrokeWidth(&requests.FPDFPageObj_GetStrokeWidth{PageObject: obj})
		must(err)
		var points [][2]float64
		bezier, closed, bezierIndex := false, false, 0
		var operators string
		flush := func() {
			if len(points) > 1 {
				pts := append([][2]float64(nil), points...)
				shape := operators
				if closed {
					// Acquire the close endpoint, then apply pinned PDFMiner
					// normalization of a redundant return-to-start line.
					pts = append(pts, pts[0])
					shape += "h"
				}
				line, rect := referencePDFClassifyPath(shape, pts)
				_, pts = referencePDFNormalizePath(shape, pts)
				pts = referencePDFLayoutPoints(pts, line, rect)
				p := referencePDFPath{linewidth: float64(width.StrokeWidth), points: pts, line: line, rect: rect, bezier: bezier, filled: paint.FillMode != enums.FPDF_FILLMODE_NONE, stroked: paint.Stroke, fill: color.FillColor, stroke: stroke.StrokeColor, x0: pts[0][0], x1: pts[0][0], y0: pts[0][1], y1: pts[0][1]}
				for _, pt := range pts {
					p.x0 = math.Min(p.x0, pt[0])
					p.x1 = math.Max(p.x1, pt[0])
					p.y0 = math.Min(p.y0, pt[1])
					p.y1 = math.Max(p.y1, pt[1])
				}
				paths = append(paths, p)
			}
			points = nil
			bezier = false
			closed = false
			bezierIndex = 0
			operators = ""
		}
		for i := 0; i < n.Count; i++ {
			seg, err := inst.FPDFPath_GetPathSegment(&requests.FPDFPath_GetPathSegment{PageObject: obj, Index: i})
			must(err)
			typ, err := inst.FPDFPathSegment_GetType(&requests.FPDFPathSegment_GetType{PathSegment: seg.PathSegment})
			must(err)
			if typ.Type == enums.FPDF_SEGMENT_MOVETO {
				flush()
			}
			point, err := inst.FPDFPathSegment_GetPoint(&requests.FPDFPathSegment_GetPoint{PathSegment: seg.PathSegment})
			must(err)
			closeFlag, err := inst.FPDFPathSegment_GetClose(&requests.FPDFPathSegment_GetClose{PathSegment: seg.PathSegment})
			must(err)
			closed = closed || closeFlag.IsClose
			// PDFMiner's pts/bbox use cubic endpoints, not control points.
			if typ.Type == enums.FPDF_SEGMENT_BEZIERTO {
				bezier = true
				bezierIndex++
				if bezierIndex%3 != 0 {
					continue
				}
			}
			switch typ.Type {
			case enums.FPDF_SEGMENT_MOVETO:
				operators += "m"
			case enums.FPDF_SEGMENT_LINETO:
				operators += "l"
			case enums.FPDF_SEGMENT_BEZIERTO:
				operators += "c"
			}
			x, y := float64(point.X), float64(point.Y)
			points = append(points, [2]float64{transform[0]*x + transform[2]*y + transform[4], transform[1]*x + transform[3]*y + transform[5]})
		}
		flush()
	}
	n, err := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: ref})
	must(err)
	for i := 0; i < n.Count; i++ {
		obj, err := inst.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: ref, Index: i})
		must(err)
		visit(obj.PageObject, pageMatrix)
	}
	if raw, e := referencePDFRawPagePaths(t, data, pageIndex); e == nil {
		paths = raw
		pageHeight = referenceRuntimePDFDimensions(t, data)[pageIndex].Height
	} else {
		t.Logf("PDF path acquisition fallback (unverified): %v", e)
	}
	result := referencePDFGeometry{paths: paths, height: pageHeight}
	if withWords {
		standardFonts := referencePDFStandardFonts(t, data)
		embeddedFonts := referencePDFEmbeddedFonts(t, data)
		fallbacks := 0
		text, err := inst.FPDFText_LoadPage(&requests.FPDFText_LoadPage{Page: ref})
		must(err)
		defer inst.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: text.TextPage})
		count, err := inst.FPDFText_CountChars(&requests.FPDFText_CountChars{TextPage: text.TextPage})
		must(err)
		rawGlyphs, rawErr := referencePDFRawEmbeddedPage(t, data, pageIndex, embeddedFonts)
		if rawErr == nil {
			for _, glyph := range rawGlyphs {
				result.chars = append(result.chars, glyph.char)
			}
		} else {
			t.Logf("raw embedded acquisition unavailable: %v", rawErr)
			for i := 0; i < count.Count; i++ {
				generated, err := inst.FPDFText_IsGenerated(&requests.FPDFText_IsGenerated{TextPage: text.TextPage, Index: i})
				must(err)
				if generated.IsGenerated {
					continue
				}
				ch, err := inst.FPDFText_GetUnicode(&requests.FPDFText_GetUnicode{TextPage: text.TextPage, Index: i})
				must(err)
				// U+0000 alone does not distinguish a valid NUL from failed decoding.
				// Use the binding's explicit status. PDFMiner preserves valid NULs;
				// on failed mapping it emits (cid:N), which this API cannot recover.
				// Keep that unsupported acquisition visible instead of fabricating N.
				mapping, err := inst.FPDFText_HasUnicodeMapError(&requests.FPDFText_HasUnicodeMapError{TextPage: text.TextPage, Index: i})
				must(err)
				if mapping.HasUnicodeMapError {
					t.Fatalf("PDF character %d needs PDFMiner CID fallback; original CID acquisition is not implemented", i)
				}
				corrected, supported, err := referencePDFStandardChar(inst, text.TextPage, i, rune(ch.Unicode), pageMatrix, pageHeight, standardFonts, embeddedFonts)
				must(err)
				if supported {
					result.chars = append(result.chars, corrected)
					continue
				}
				fallbacks++
				box, err := inst.FPDFText_GetLooseCharBox(&requests.FPDFText_GetLooseCharBox{TextPage: text.TextPage, Index: i})
				must(err)
				transform, err := inst.FPDFText_GetMatrix(&requests.FPDFText_GetMatrix{TextPage: text.TextPage, Index: i})
				must(err)
				m, r := transform.Matrix, box.Rect
				// Candidate orientation adapter. PDFium's effective character matrix
				// and LTChar's separate horizontal scaling still require equivalence
				// review, especially mirrored/vertical writing. Do not count as equivalence.
				effective := referencePDFComposeMatrix(pageMatrix, matrix{float64(m.A), float64(m.B), float64(m.C), float64(m.D), float64(m.E), float64(m.F)})
				upright := effective[0]*effective[3] > 0 && effective[1]*effective[2] <= 0
				result.chars = append(result.chars, referencePDFDirectedChar{
					char:    referencePDFTableChar{string(rune(ch.Unicode)), referencePDFPageBox(pageMatrix, pageHeight, [4]float64{float64(r.Left), float64(r.Bottom), float64(r.Right), float64(r.Top)})},
					upright: upright,
				})
			}
		}
		if fallbacks > 0 {
			t.Logf("PDF character acquisition: %d characters retain unverified PDFium loose bounds (non-standard font/encoding/size)", fallbacks)
		}
		for _, word := range referencePDFDefaultWords(result.chars) {
			result.words = append(result.words, referencePDFWord{
				text: word.text, x0: word.box[0], top: word.box[1], x1: word.box[2], bottom: word.box[3],
			})
		}
	}
	return result
}

func referencePDFCurveRectCount(paths []referencePDFPath) int {
	count := 0
	for _, p := range paths {
		if !p.line {
			count++
		}
	}
	return count
}

// Original is_sparkline_mark checks both colors regardless of paint mode.
func referenceIsSparklineMark(mark referencePDFPath) bool {
	for _, color := range [][]float64{referencePDFPathColor(mark, false), referencePDFPathColor(mark, true)} {
		if len(color) != 3 {
			continue
		}
		for _, want := range [][3]uint{{0x24, 0x5A, 0x81}, {0xCC, 0x33, 0x33}} {
			if math.Abs(color[0]-float64(want[0])/255) < .01 && math.Abs(color[1]-float64(want[1])/255) < .01 && math.Abs(color[2]-float64(want[2])/255) < .01 {
				return true
			}
		}
	}
	return false
}

// Edges use top-down page coordinates. Rectangles contribute their four
// bounding sides; curves contribute consecutive endpoint pairs, as page.edges.
type referencePDFEdge struct{ x0, x1, top, bottom float64 }

func referencePDFEdges(g referencePDFGeometry) []referencePDFEdge {
	var edges []referencePDFEdge
	add := func(a, b [2]float64) {
		edges = append(edges, referencePDFEdge{math.Min(a[0], b[0]), math.Max(a[0], b[0]), g.height - math.Max(a[1], b[1]), g.height - math.Min(a[1], b[1])})
	}
	for _, p := range g.paths {
		if p.line {
			// PDFMiner ml/mlh both produce one LTLine. pdfplumber's
			// line_to_edge returns that one bounding edge, not the closing
			// return segment retained in our raw path points.
			edges = append(edges, referencePDFEdge{p.x0, p.x1, g.height - p.y1, g.height - p.y0})
		} else if p.rect {
			// Preserve rect_to_edges order: top, bottom, left, right.
			add([2]float64{p.x0, p.y1}, [2]float64{p.x1, p.y1})
			// Pinned rect_to_edges adds the rectangle height to its top for
			// the bottom edge's top, but retains its independently acquired bottom.
			// Reassociation changes low bits even on ordinary decimal coordinates.
			edges = append(edges, referencePDFEdge{p.x0, p.x1, (g.height - p.y1) + (p.y1 - p.y0), g.height - p.y0})
			add([2]float64{p.x0, p.y0}, [2]float64{p.x0, p.y1})
			add([2]float64{p.x1, p.y0}, [2]float64{p.x1, p.y1})
		} else {
			for i := 1; i < len(p.points); i++ {
				add(p.points[i-1], p.points[i])
			}
		}
	}
	return edges
}
func referencePDFRequireWord(t *testing.T, g referencePDFGeometry, text string) referencePDFWord {
	t.Helper()
	for _, w := range g.words {
		if w.text == text {
			return w
		}
	}
	t.Fatalf("missing word %q: %+v", text, g.words)
	return referencePDFWord{}
}
