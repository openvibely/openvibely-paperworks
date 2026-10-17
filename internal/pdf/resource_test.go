package pdf

import (
	"context"
	"github.com/klippa-app/go-pdfium/requests"
	"math"
	"testing"
	"time"
)

func TestEngineWaitCancellation(t *testing.T) {
	_, done, e := engine(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, e = engine(ctx)
	if e != context.DeadlineExceeded {
		t.Fatal(e)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation blocked behind renderer")
	}
}
func TestRenderAllocationLimits(t *testing.T) {
	for _, v := range []struct {
		w, h float64
		o    RenderOptions
	}{{0, 100, RenderOptions{DPI: 72}}, {math.NaN(), 100, RenderOptions{DPI: 72}}, {math.Inf(1), 100, RenderOptions{DPI: 72}}, {20000, 20000, RenderOptions{DPI: 600}}, {10000, 10000, RenderOptions{MaxPixels: 10000}}} {
		if renderDimensions(v.w, v.h, v.o) == nil {
			t.Fatal(v)
		}
	}
	if e := renderDimensions(612, 792, RenderOptions{DPI: 300}); e != nil {
		t.Fatal(e)
	}
}

// Borrowed-instance cancellation must invalidate the worker, not just cancel a
// pool wait; subsequent clients must receive a live replacement worker.
func TestEngineCancellationReplacesWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	inst, done, e := engine(ctx)
	if e != nil {
		t.Fatal(e)
	}
	data := formPDF()
	doc, e := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if e != nil {
		done()
		cancel()
		t.Fatal(e)
	}
	cancel()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	waiting := true
	for waiting {
		select {
		case <-deadline:
			done()
			t.Fatal("worker was not interrupted")
		case <-tick.C:
			_, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
			if err != nil {
				waiting = false
			}
		}
	}
	// done joins the cancellation watcher. Either cancellation wins, or release
	// wins after work has completed; both paths must release the pool slot safely.
	done()
	next, release, e := engine(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if next == inst {
		t.Fatal("returned same borrowed instance")
	}
	doc, e = next.OpenDocument(&requests.OpenDocument{File: &data})
	if e != nil {
		t.Fatal("replacement worker unusable", e)
	}
	if _, e = next.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}); e != nil {
		t.Fatal(e)
	}
}
