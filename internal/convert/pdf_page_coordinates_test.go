package convert

// UNRUN source contract: pdfminer.six 20231228 PDFPageInterpreter.process_page
// and PDFLayoutAnalyzer.begin_page. The caller supplies normalized page.rotate,
// not PDFium's quarter-turn enum. Do not apply this to already rotated boxes.
import (
	"math"
	"testing"
)

type referencePDFMatrix [6]float64

func referencePDFTransformPoint(m referencePDFMatrix, p [2]float64) [2]float64 {
	return [2]float64{m[0]*p[0] + m[2]*p[1] + m[4], m[1]*p[0] + m[3]*p[1] + m[5]}
}

func referencePDFComposeMatrix(parent, child referencePDFMatrix) referencePDFMatrix {
	p, q := parent, child
	return referencePDFMatrix{p[0]*q[0] + p[2]*q[1], p[1]*q[0] + p[3]*q[1], p[0]*q[2] + p[2]*q[3], p[1]*q[2] + p[3]*q[3], p[0]*q[4] + p[2]*q[5] + p[4], p[1]*q[4] + p[3]*q[5] + p[5]}
}

func referencePDFMinerPageCoordinates(media [4]float64, rotation int) (referencePDFMatrix, float64, float64) {
	x0, y0, x1, y1 := media[0], media[1], media[2], media[3]
	m := referencePDFMatrix{1, 0, 0, 1, -x0, -y0}
	switch rotation {
	case 90:
		m = referencePDFMatrix{0, -1, 1, 0, -y0, x1}
	case 180:
		m = referencePDFMatrix{-1, 0, 0, -1, x1, y1}
	case 270:
		m = referencePDFMatrix{0, 1, -1, 0, y1, -x0}
	}
	a, b := referencePDFTransformPoint(m, [2]float64{x0, y0}), referencePDFTransformPoint(m, [2]float64{x1, y1})
	return m, math.Abs(b[0] - a[0]), math.Abs(b[1] - a[1])
}

func TestReferencePDFMinerPageCoordinateContracts(t *testing.T) {
	for _, tc := range []struct {
		name          string
		rotation      int
		point         [2]float64
		width, height float64
	}{
		{"translated", 0, [2]float64{30, 40}, 200, 100},
		{"quarter-turn", 90, [2]float64{40, 170}, 100, 200},
		{"half-turn", 180, [2]float64{170, 60}, 200, 100},
		{"three-quarter-turn", 270, [2]float64{60, 30}, 100, 200},
		{"other-angle-source-fallback", 45, [2]float64{30, 40}, 200, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, w, h := referencePDFMinerPageCoordinates([4]float64{10, 20, 210, 120}, tc.rotation)
			if got := referencePDFTransformPoint(m, [2]float64{40, 60}); got != tc.point {
				t.Fatalf("point=%v want %v", got, tc.point)
			}
			if w != tc.width || h != tc.height {
				t.Fatalf("size=%v,%v want %v,%v", w, h, tc.width, tc.height)
			}
		})
	}
}

func TestReferencePDFNestedFormCoordinateContract(t *testing.T) {
	// Child scale/translation, then enclosing Form quarter-turn, then page
	// origin translation. Reversing any multiplication changes the expected point.
	page, _, _ := referencePDFMinerPageCoordinates([4]float64{10, 20, 210, 120}, 0)
	form := referencePDFMatrix{0, 1, -1, 0, 100, 50}
	child := referencePDFMatrix{2, 0, 0, 3, 4, 5}
	combined := referencePDFComposeMatrix(referencePDFComposeMatrix(page, form), child)
	if got := referencePDFTransformPoint(combined, [2]float64{7, 11}); got != ([2]float64{52, 48}) {
		t.Fatalf("nested point=%v want [52 48]", got)
	}
}

// Transform an already acquired user-space box into PDFMiner's top-down page
// frame. This only changes coordinates; it cannot turn loose font/glyph bounds
// into LTChar advance/descent bounds. Page matrices are orthogonal quarter turns.
func referencePDFPageBox(page referencePDFMatrix, height float64, box [4]float64) referencePDFCell {
	a := referencePDFTransformPoint(page, [2]float64{box[0], box[1]})
	b := referencePDFTransformPoint(page, [2]float64{box[2], box[3]})
	return referencePDFCell{math.Min(a[0], b[0]), height - math.Max(a[1], b[1]), math.Max(a[0], b[0]), height - math.Min(a[1], b[1])}
}

// UNRUN. Literal expectations include nonzero MediaBox origins. The smaller
// CropBox is deliberately absent: PDFMiner begin_page sizes from MediaBox.
func TestReferencePDFPageBoxCoordinateContracts(t *testing.T) {
	for _, tc := range []struct {
		rotation int
		want     referencePDFCell
	}{
		{0, referencePDFCell{30, 50, 40, 60}},
		{90, referencePDFCell{40, 30, 50, 40}},
		{180, referencePDFCell{160, 40, 170, 50}},
		{270, referencePDFCell{50, 160, 60, 170}},
	} {
		t.Run(map[int]string{0: "origin", 90: "quarter", 180: "half", 270: "three-quarter"}[tc.rotation], func(t *testing.T) {
			page, _, height := referencePDFMinerPageCoordinates([4]float64{10, 20, 210, 120}, tc.rotation)
			got := referencePDFPageBox(page, height, [4]float64{40, 60, 50, 70})
			if got != tc.want {
				t.Fatalf("box=%v want %v", got, tc.want)
			}
		})
	}
}

// UNRUN: source-derived separation of a Form's invocation matrix from its
// dictionary Matrix. CPDF_ContentParser incorporates the dictionary Matrix in
// child object matrices; CPDF_StreamContentParser::AddForm stores the invocation
// CTM on the Form object. Traversal must not apply the dictionary Matrix again.
// This is an algebra contract, not execution evidence for the embedded engine.
func TestReferencePDFFormDictionaryMatrixAppliedOnce(t *testing.T) {
	page, _, _ := referencePDFMinerPageCoordinates([4]float64{10, 20, 210, 120}, 0)
	invocation := referencePDFMatrix{0, 1, -1, 0, 100, 50}
	dictionary := referencePDFMatrix{2, 0, 0, 3, 4, 5}
	local := referencePDFMatrix{1, 0, 0, 1, 7, 11}
	acquiredChild := referencePDFComposeMatrix(dictionary, local)
	traversed := referencePDFComposeMatrix(referencePDFComposeMatrix(page, invocation), acquiredChild)
	if got := referencePDFTransformPoint(traversed, [2]float64{1, 2}); got != ([2]float64{46, 50}) {
		t.Fatalf("Form point=%v; want [46 50] with dictionary Matrix applied once", got)
	}
}
