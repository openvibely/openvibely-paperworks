package convert

// Bounded PDFMiner 20260107 path acquisition. No XObjects, custom color spaces,
// ExtGState or inline images: unsupported streams discard the entire candidate.
import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/klippa-app/go-pdfium/structs"
	documentpdf "github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func referencePDFRawPagePaths(t *testing.T, data []byte, index int) ([]referencePDFPath, error) {
	t.Helper()
	c, e := documentpdf.Load(context.Background(), data)
	if e != nil {
		return nil, e
	}
	p, _, a, e := c.PageDict(context.Background(), index+1, false)
	if e != nil {
		return nil, e
	}
	raw, e := c.PageContent(p, index+1)
	if e != nil {
		return nil, e
	}
	if a.MediaBox == nil {
		return nil, fmt.Errorf("missing MediaBox")
	}
	b := a.MediaBox
	m, _, _ := referencePDFMinerPageCoordinates([4]float64{b.LL.X, b.LL.Y, b.UR.X, b.UR.Y}, ((a.Rotate%360)+360)%360)
	return referencePDFRawPaths(string(raw), m)
}

func referencePDFRawPaths(input string, page referencePDFMatrix) ([]referencePDFPath, error) {
	type state struct {
		matrix         referencePDFMatrix
		width          float64
		fill, stroke   [3]float64
		fillN, strokeN int
	}
	st := state{matrix: page, fillN: 1, strokeN: 1}
	var stack []state
	type segment struct {
		op    string
		point [2]float64
	}
	var current []segment
	var result []referencePDFPath
	var args []types.Object
	inText := false
	marked := 0
	fail := func(op string) ([]referencePDFPath, error) {
		return nil, fmt.Errorf("raw paths: unsupported or malformed %s", op)
	}
	number := func(o types.Object) (float64, bool) {
		switch v := o.(type) {
		case types.Float:
			return float64(v), true
		case types.Integer:
			return float64(v), true
		}
		return 0, false
	}
	nums := func(n int) ([]float64, bool) {
		if len(args) != n {
			return nil, false
		}
		v := make([]float64, n)
		for i, a := range args {
			var ok bool
			v[i], ok = number(a)
			if !ok {
				return nil, false
			}
		}
		return v, true
	}
	color := func(v [3]float64) structs.FPDF_COLOR {
		return structs.FPDF_COLOR{R: uint(math.Round(v[0] * 255)), G: uint(math.Round(v[1] * 255)), B: uint(math.Round(v[2] * 255)), A: 255}
	}
	paint := func(stroke, fill bool) {
		for start := 0; start < len(current); {
			end := start + 1
			for end < len(current) && current[end].op != "m" {
				end++
			}
			if current[start].op == "m" && end-start > 1 {
				shape := ""
				points := make([][2]float64, 0, end-start)
				bezier := false
				for _, s := range current[start:end] {
					shape += s.op
					p := s.point
					if s.op == "h" {
						p = current[start].point
					}
					points = append(points, referencePDFTransformPoint(st.matrix, p))
					bezier = bezier || s.op == "c"
				}
				line, rect := referencePDFClassifyPath(shape, points)
				_, points = referencePDFNormalizePath(shape, points)
				points = referencePDFLayoutPoints(points, line, rect)
				p := referencePDFPath{line: line, rect: rect, bezier: bezier, linewidth: st.width, points: points, filled: fill, stroked: stroke, fill: color(st.fill), stroke: color(st.stroke), x0: points[0][0], x1: points[0][0], y0: points[0][1], y1: points[0][1]}
				p.exactFill = append([]float64(nil), st.fill[:st.fillN]...)
				p.exactStroke = append([]float64(nil), st.stroke[:st.strokeN]...)
				for _, q := range points {
					p.x0 = math.Min(p.x0, q[0])
					p.x1 = math.Max(p.x1, q[0])
					p.y0 = math.Min(p.y0, q[1])
					p.y1 = math.Max(p.y1, q[1])
				}
				result = append(result, p)
			}
			start = end
		}
		current = nil
	}
	for {
		input = strings.TrimLeft(input, "\x00\t\n\f\r ")
		if input == "" {
			break
		}
		if input[0] == '%' {
			i := strings.IndexAny(input, "\r\n")
			if i < 0 {
				break
			}
			input = input[i:]
			continue
		}
		if strings.ContainsRune("+-.0123456789", rune(input[0])) {
			i := strings.IndexAny(input, "\x00\t\n\f\r /()<>[]%")
			if i < 0 {
				i = len(input)
			}
			token := input[:i]
			if strings.IndexFunc(token, func(r rune) bool { return !strings.ContainsRune("+-.0123456789", r) }) >= 0 {
				return fail("number")
			}
			v, e := strconv.ParseFloat(token, 64)
			if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return fail("number")
			}
			args = append(args, types.Float(v))
			input = input[i:]
			continue
		}
		if strings.ContainsRune("/([<", rune(input[0])) {
			before := len(input)
			v, e := model.ParseObjectWithPolicy(context.Background(), &input, 0, model.ValidationStrict)
			if e != nil || len(input) >= before {
				return fail("operand")
			}
			args = append(args, v.Object)
			continue
		}
		i := strings.IndexAny(input, "\x00\t\n\f\r /()<>[]%")
		if i < 0 {
			i = len(input)
		}
		if i == 0 {
			return fail("delimiter")
		}
		op := input[:i]
		input = input[i:]
		switch op {
		case "q":
			if len(args) != 0 || inText {
				return fail(op)
			}
			stack = append(stack, st)
		case "Q":
			if len(args) != 0 || inText || len(stack) == 0 {
				return fail(op)
			}
			st = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case "cm":
			v, ok := nums(6)
			if !ok || inText {
				return fail(op)
			}
			st.matrix = referencePDFComposeMatrix(st.matrix, referencePDFMatrix{v[0], v[1], v[2], v[3], v[4], v[5]})
		case "w":
			v, ok := nums(1)
			if !ok {
				return fail(op)
			}
			st.width = v[0]
		case "rg", "RG", "g", "G":
			n := 3
			if op == "g" || op == "G" {
				n = 1
			}
			v, ok := nums(n)
			if !ok {
				return fail(op)
			}
			rgb := [3]float64{v[0], v[0], v[0]}
			if n == 3 {
				copy(rgb[:], v)
			}
			for _, v := range rgb {
				if v < 0 || v > 1 {
					return fail(op)
				}
			}
			if op == "rg" || op == "g" {
				st.fill = rgb
				st.fillN = n
			} else {
				st.stroke = rgb
				st.strokeN = n
			}
		case "m", "l", "c", "v", "y", "re":
			n := 2
			if op == "c" {
				n = 6
			}
			if op == "v" || op == "y" || op == "re" {
				n = 4
			}
			v, ok := nums(n)
			if !ok || inText {
				return fail(op)
			}
			if op != "m" && op != "re" && len(current) == 0 {
				return fail(op)
			}
			if op == "re" {
				x, y, w, h := v[0], v[1], v[2], v[3]
				current = append(current, segment{"m", [2]float64{x, y}}, segment{"l", [2]float64{x + w, y}}, segment{"l", [2]float64{x + w, y + h}}, segment{"l", [2]float64{x, y + h}}, segment{op: "h"})
			} else {
				current = append(current, segment{op, [2]float64{v[n-2], v[n-1]}})
			}
		case "h":
			if len(args) != 0 || len(current) == 0 || inText {
				return fail(op)
			}
			current = append(current, segment{op: "h"})
		case "S", "s", "f", "f*", "B", "B*", "b", "b*", "n":
			if len(args) != 0 || inText {
				return fail(op)
			}
			if op == "n" {
				current = nil
				break
			}
			if op == "s" || op == "b" || op == "b*" {
				if len(current) == 0 {
					return fail(op)
				}
				current = append(current, segment{op: "h"})
			}
			paint(op != "f" && op != "f*", op != "S" && op != "s")
		// PDFMiner's path output does not apply clipping, dash, cap or join styles.
		case "W", "W*":
			if len(args) != 0 {
				return fail(op)
			}
		case "J", "j", "M":
			if _, ok := nums(1); !ok {
				return fail(op)
			}
		case "d":
			if len(args) != 2 {
				return fail(op)
			}
			if _, ok := args[0].(types.Array); !ok {
				return fail(op)
			}
			if _, ok := number(args[1]); !ok {
				return fail(op)
			}
		case "BMC":
			if len(args) != 1 {
				return fail(op)
			}
			if _, ok := args[0].(types.Name); !ok {
				return fail(op)
			}
			marked++
		case "BDC":
			if len(args) != 2 {
				return fail(op)
			}
			if _, ok := args[0].(types.Name); !ok {
				return fail(op)
			}
			marked++
		case "EMC":
			if len(args) != 0 || marked == 0 {
				return fail(op)
			}
			marked--
		case "BT":
			if len(args) != 0 || inText {
				return fail(op)
			}
			inText = true
		case "ET":
			if len(args) != 0 || !inText {
				return fail(op)
			}
			inText = false
		// Text placement does not affect PDFMiner paint_path; unknown operators still fail.
		case "Tm", "Td", "TD", "Tc", "Tw", "Tz", "TL", "Tr", "Ts":
			n := 1
			if op == "Tm" {
				n = 6
			}
			if op == "Td" || op == "TD" {
				n = 2
			}
			if _, ok := nums(n); !ok || !inText {
				return fail(op)
			}
		case "Tf":
			if !inText || len(args) != 2 {
				return fail(op)
			}
			if _, ok := args[0].(types.Name); !ok {
				return fail(op)
			}
			if _, ok := number(args[1]); !ok {
				return fail(op)
			}
		case "T*":
			if !inText || len(args) != 0 {
				return fail(op)
			}
		case "Tj", "TJ", "'", "\"":
			if !inText {
				return fail(op)
			}
			if op == "\"" {
				if len(args) != 3 {
					return fail(op)
				}
				for _, a := range args[:2] {
					if _, ok := number(a); !ok {
						return fail(op)
					}
				}
				args = args[2:]
			}
			if len(args) != 1 {
				return fail(op)
			}
			items := args
			if op == "TJ" {
				var ok bool
				items, ok = args[0].(types.Array)
				if !ok {
					return fail(op)
				}
			}
			for _, a := range items {
				switch a.(type) {
				case types.StringLiteral, types.HexLiteral:
				default:
					if _, ok := number(a); !ok || op != "TJ" {
						return fail(op)
					}
				}
			}

		default:
			return fail(op)
		}
		args = nil
	}
	if len(args) != 0 || len(stack) != 0 || marked != 0 || inText {
		return fail("unfinished stream")
	}
	return result, nil
}

func TestReferencePDFRawPathBoundaries(t *testing.T) {
	identity := referencePDFMatrix{1, 0, 0, 1, 0, 0}
	for _, input := range []string{"0 0 m 1 1 l S /X Do", "0 0 m 1 1 l S /GS gs", "q 1 2 m 3 4 l S", "0 0 m 1 1 l S BI", "0 0 m 1 1 l S 0 0 0 RGjunk"} {
		if got, e := referencePDFRawPaths(input, identity); e == nil || got != nil {
			t.Fatalf("accepted partial page %q: %+v %v", input, got, e)
		}
	}
	p, e := referencePDFRawPaths("q 1 0 0 1 .1 .2 cm .25 w /Artifact BMC 0 0 2 3 re S EMC Q", identity)
	if e != nil || len(p) != 1 {
		t.Fatal(p, e)
	}
	if !p[0].rect || p[0].x0 != .1 || p[0].y0 != .2 || p[0].x1 != 2.1 || p[0].y1 != 3.2 || p[0].linewidth != .25 {
		t.Fatal(p)
	}
}

// Preserve the original color precision and gray/RGB arity. The PDFium
// fallback remains quantized and is never treated as proven raw acquisition.
func referencePDFPathColor(p referencePDFPath, stroke bool) []float64 {
	if stroke {
		if p.exactStroke != nil {
			return p.exactStroke
		}
		return []float64{float64(p.stroke.R) / 255, float64(p.stroke.G) / 255, float64(p.stroke.B) / 255}
	}
	if p.exactFill != nil {
		return p.exactFill
	}
	return []float64{float64(p.fill.R) / 255, float64(p.fill.G) / 255, float64(p.fill.B) / 255}
}
