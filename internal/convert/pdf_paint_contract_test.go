package convert

import (
	"fmt"
	"reflect"
	"testing"
)

// UNRUN source port of PDFMiner 20231228 PDFPageInterpreter.do_{S,s,f,F,
// f_a,B,B_a,b,b_a,n,W,W_a}. This consumes ORIGINAL operators; normalized
// PDFium path segments cannot supply the explicit/implicit close distinction.
// It is not yet connected to a content-stream reader.
type referencePDFPaintState struct {
	shape  string
	points [][2]float64
}
type referencePDFPaintEvent struct {
	shape                 string
	points                [][2]float64
	stroke, fill, evenOdd bool
}

func (p *referencePDFPaintState) apply(op string) (*referencePDFPaintEvent, error) {
	switch op {
	case "W", "W*":
		// PDFMiner's layout interpreter does not apply clipping to recorded paths.
		return nil, nil
	case "n":
		p.shape, p.points = "", nil
		return nil, nil
	case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*":
	default:
		return nil, fmt.Errorf("unsupported path paint operator %q", op)
	}
	if op == "s" || op == "b" || op == "b*" {
		// h closes the latest subpath, not the first subpath in a compound path.
		start := -1
		for i, c := range p.shape {
			if c == 'm' {
				start = i
			}
		}
		p.shape += "h"
		if start >= 0 {
			p.points = append(p.points, p.points[start])
		}
	}
	e := &referencePDFPaintEvent{
		shape: p.shape, points: append([][2]float64(nil), p.points...),
		stroke:  op == "S" || op == "s" || op == "B" || op == "B*" || op == "b" || op == "b*",
		fill:    op != "S" && op != "s",
		evenOdd: op == "f*" || op == "B*" || op == "b*",
	}
	p.shape, p.points = "", nil
	return e, nil
}

// UNRUN. Expected modes and closure are literal source expectations, including
// F's obsolete alias and the absence of an added point for implicitly closed fill.
func TestReferencePDFPaintOperatorContracts(t *testing.T) {
	for _, tc := range []struct {
		op                           string
		stroke, fill, evenOdd, close bool
	}{
		{"S", true, false, false, false}, {"s", true, false, false, true},
		{"f", false, true, false, false}, {"F", false, true, false, false}, {"f*", false, true, true, false},
		{"B", true, true, false, false}, {"B*", true, true, true, false},
		{"b", true, true, false, true}, {"b*", true, true, true, true},
	} {
		t.Run(tc.op, func(t *testing.T) {
			a, b, c := [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{10, 10}
			p := referencePDFPaintState{"mll", [][2]float64{a, b, c}}
			e, err := p.apply(tc.op)
			if err != nil || e == nil {
				t.Fatalf("paint: %v %v", e, err)
			}
			shape, want := "mll", [][2]float64{a, b, c}
			if tc.close {
				shape = "mllh"
				want = append(want, a)
			}
			if e.shape != shape || !reflect.DeepEqual(e.points, want) || e.stroke != tc.stroke || e.fill != tc.fill || e.evenOdd != tc.evenOdd {
				t.Fatalf("%s event=%+v; want shape=%s points=%v stroke=%v fill=%v evenOdd=%v", tc.op, e, shape, want, tc.stroke, tc.fill, tc.evenOdd)
			}
			if p.shape != "" || len(p.points) != 0 {
				t.Fatalf("paint did not clear current path: %+v", p)
			}
		})
	}
}

func TestReferencePDFClipAndDiscardContracts(t *testing.T) {
	for _, clip := range []string{"W", "W*"} {
		for _, end := range []string{"n", "S"} {
			t.Run(clip+"/"+end, func(t *testing.T) {
				p := referencePDFPaintState{"ml", [][2]float64{{0, 0}, {10, 0}}}
				before := referencePDFPaintState{p.shape, append([][2]float64(nil), p.points...)}
				e, err := p.apply(clip)
				if err != nil || e != nil || !reflect.DeepEqual(p, before) {
					t.Fatalf("clip changed layout path: %+v, %v", p, err)
				}
				e, err = p.apply(end)
				if err != nil {
					t.Fatal(err)
				}
				if end == "n" && e != nil {
					t.Fatalf("unpainted path emitted: %+v", e)
				}
				if end == "S" && (e == nil || e.shape != "ml" || !e.stroke) {
					t.Fatalf("clipped stroke lost: %+v", e)
				}
				if p.shape != "" || len(p.points) != 0 {
					t.Fatalf("end did not clear path: %+v", p)
				}
			})
		}
	}
}

func TestReferencePDFCloseLastSubpathContract(t *testing.T) {
	p := referencePDFPaintState{"mlml", [][2]float64{{0, 0}, {10, 0}, {20, 20}, {30, 20}}}
	e, err := p.apply("b")
	want := [][2]float64{{0, 0}, {10, 0}, {20, 20}, {30, 20}, {20, 20}}
	if err != nil || e == nil || e.shape != "mlmlh" || !reflect.DeepEqual(e.points, want) {
		t.Fatalf("close last subpath: %+v %v", e, err)
	}
}
