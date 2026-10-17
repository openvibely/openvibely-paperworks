package convert

import "testing"

// UNRUN source contracts for pdfminer.six 20231228 PDFPageInterpreter
// init_state/get_current_state/do_q/do_Q/do_cm/do_Do and PDFGraphicState.
// This deliberately models only CTM, linewidth and current path. It is not an
// original-stream reader or a complete graphics/text state implementation.
type referencePDFGraphicsSnapshot struct {
	ctm       referencePDFMatrix
	linewidth float64
}
type referencePDFGraphicsContract struct {
	referencePDFGraphicsSnapshot
	stack []referencePDFGraphicsSnapshot
	path  referencePDFPaintState
}

func referencePDFNewGraphicsContract(ctm referencePDFMatrix) referencePDFGraphicsContract {
	// PDFMiner's default linewidth is zero, not the PDF specification's one.
	return referencePDFGraphicsContract{referencePDFGraphicsSnapshot: referencePDFGraphicsSnapshot{ctm: ctm}}
}
func (p *referencePDFGraphicsContract) save() {
	p.stack = append(p.stack, p.referencePDFGraphicsSnapshot)
}
func (p *referencePDFGraphicsContract) restore() {
	if len(p.stack) != 0 {
		p.referencePDFGraphicsSnapshot = p.stack[len(p.stack)-1]
		p.stack = p.stack[:len(p.stack)-1]
	}
}
func (p *referencePDFGraphicsContract) concat(m referencePDFMatrix) {
	p.ctm = referencePDFComposeMatrix(p.ctm, m)
}
func (p *referencePDFGraphicsContract) form(m referencePDFMatrix) referencePDFGraphicsContract {
	// do_Do duplicates the interpreter, then render_contents calls init_state.
	// The parent graphics state is not passed to the new interpreter.
	return referencePDFNewGraphicsContract(referencePDFComposeMatrix(p.ctm, m))
}

func TestReferencePDFGraphicsStateContracts(t *testing.T) {
	identity := referencePDFMatrix{1, 0, 0, 1, 0, 0}
	t.Run("initial-width", func(t *testing.T) {
		p := referencePDFNewGraphicsContract(identity)
		if p.linewidth != 0 || p.ctm != identity || len(p.stack) != 0 || p.path.shape != "" {
			t.Fatalf("initial state=%+v", p)
		}
	})
	t.Run("restore-does-not-restore-current-path", func(t *testing.T) {
		p := referencePDFNewGraphicsContract(identity)
		p.linewidth = 2
		p.path = referencePDFPaintState{"m", [][2]float64{{1, 2}}}
		p.save()
		p.linewidth = 9
		p.concat(referencePDFMatrix{2, 0, 0, 3, 4, 5})
		p.path.shape += "l"
		p.path.points = append(p.path.points, [2]float64{8, 9})
		p.restore()
		if p.ctm != identity || p.linewidth != 2 || len(p.stack) != 0 || p.path.shape != "ml" || len(p.path.points) != 2 || p.path.points[1] != ([2]float64{8, 9}) {
			t.Fatalf("restored state/path=%+v", p)
		}
	})
	t.Run("empty-restore-is-noop", func(t *testing.T) {
		p := referencePDFNewGraphicsContract(identity)
		p.linewidth = 7
		p.restore()
		if p.linewidth != 7 || p.ctm != identity {
			t.Fatalf("unmatched Q=%+v", p)
		}
	})
	t.Run("form-resets-width-and-path-preserves-parent", func(t *testing.T) {
		p := referencePDFNewGraphicsContract(referencePDFMatrix{0, 1, -1, 0, 100, 50})
		p.linewidth = 9
		p.path = referencePDFPaintState{"ml", [][2]float64{{1, 2}, {3, 4}}}
		p.save()
		child := p.form(referencePDFMatrix{2, 0, 0, 3, 4, 5})
		if child.linewidth != 0 || len(child.stack) != 0 || child.path.shape != "" || len(child.path.points) != 0 {
			t.Fatalf("Form inherited parent state=%+v", child)
		}
		if got := referencePDFTransformPoint(child.ctm, [2]float64{7, 11}); got != ([2]float64{62, 68}) {
			t.Fatalf("Form point=%v want [62 68]", got)
		}
		child.linewidth = 4
		if p.linewidth != 9 || len(p.stack) != 1 || p.path.shape != "ml" {
			t.Fatalf("Form changed parent=%+v", p)
		}
	})
	t.Run("cm-order-and-unscaled-linewidth", func(t *testing.T) {
		p := referencePDFNewGraphicsContract(identity)
		p.linewidth = 2
		p.concat(referencePDFMatrix{1, 0, 0, 1, 10, 20})
		p.concat(referencePDFMatrix{2, 0, 0, 3, 0, 0})
		if got := referencePDFTransformPoint(p.ctm, [2]float64{4, 5}); got != ([2]float64{18, 35}) {
			t.Fatalf("cm point=%v want [18 35]", got)
		}
		// PDFLayoutAnalyzer.paint_path passes gstate.linewidth unchanged.
		if p.linewidth != 2 {
			t.Fatalf("linewidth scaled to %v", p.linewidth)
		}
	})
}
