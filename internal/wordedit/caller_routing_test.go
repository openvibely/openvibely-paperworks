package wordedit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// These are the original helper operands, captured while all 57 unchanged
// methods run. They check caller wiring, not application results in place of
// the original assertions. Supplemental tests keep their independent helpers.
type referenceWordCall struct {
	Name           string `json:"name"`
	Operation      string `json:"operation"`
	DefaultSource  bool   `json:"default_source"`
	OriginalSource bool   `json:"original_source"`
	Success        bool   `json:"success"`
}
type referenceWordRoute struct {
	mu             sync.Mutex
	root, original string
	calls          []referenceWordCall
	next           int
}

var referenceWordRoutes sync.Map

func referenceWordCaller(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/reference-mirror/word-caller-routing.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows map[string]struct {
		Calls []referenceWordCall `json:"calls"`
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	row, ok := rows[t.Name()]
	if !ok {
		t.Fatal("missing original caller routing", t.Name())
	}
	source := base(t)
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source = filepath.Join(root, "original.docx")
	if err = os.WriteFile(source, original, 0600); err != nil {
		t.Fatal(err)
	}
	registerReferenceOriginal(t, source)
	state := &referenceWordRoute{root: root, original: source, calls: row.Calls}
	referenceWordRoutes.Store(t.Name(), state)
	t.Cleanup(func() {
		defer referenceWordRoutes.Delete(t.Name())
		if state.next != len(state.calls) {
			t.Errorf("original calls consumed %d/%d", state.next, len(state.calls))
		}
	})
}

func referenceWordRoutedEdit(t *testing.T, src string, spec map[string]any) (string, error, bool) {
	t.Helper()
	value, ok := referenceWordRoutes.Load(strings.Split(t.Name(), "/")[0])
	if !ok {
		return "", nil, false
	}
	state := value.(*referenceWordRoute)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.next >= len(state.calls) {
		t.Fatal("extra edit absent from original helper sequence")
	}
	call := state.calls[state.next]
	state.next++
	if spec["operation"] != call.Operation {
		t.Fatalf("original call %d operation: got %v want %s", state.next, spec["operation"], call.Operation)
	}
	if call.OriginalSource {
		src = state.original
	}
	out, err := referenceNamedEdit(t, state.root, call.Name, src, spec)
	if (err == nil) != call.Success {
		t.Errorf("original call %d %s success: got %v want %v (%v)", state.next, call.Name, err == nil, call.Success, err)
	}
	return out, err, true
}
