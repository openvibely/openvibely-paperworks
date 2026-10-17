package pptx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTemplatePresentationWholeArchive(t *testing.T) {
	base := "../../testdata/document-examples/document_presentations/examples"
	raw, e := os.ReadFile(filepath.Join(base, "revenue-slide.json"))
	if e != nil {
		t.Fatal(e)
	}
	var s Spec
	if e = json.Unmarshal(raw, &s); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "out.pptx")
	clock := func() time.Time { return time.Date(2024, 2, 3, 4, 5, 6, 0, time.Local) }
	// Core properties use UTC; choose the instant with the controlled UTC wall time.
	t.Setenv("TZ", "UTC")
	old := time.Local
	time.Local = time.UTC
	defer func() { time.Local = old }()
	if e = createWithClock(context.Background(), s, base, out, clock); e != nil {
		t.Fatal(e)
	}
	raw, e = os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	want, e := os.ReadFile("testdata/presentation-fixed.sha256")
	if e != nil {
		t.Fatal(e)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != strings.TrimSpace(string(want)) {
		t.Fatalf("whole PPTX %s want %s", got, want)
	}
}
