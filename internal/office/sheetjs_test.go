package office

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// This independent-reader contract intentionally runs SheetJS, exactly as the
// original test does. All setup and assertions are native Go; the app does not
// depend on Node or SheetJS. Set PAPERWORKS_TEST_SHEETJS for exhaustive audits.
func TestReferenceIndependentSheetJSComments(t *testing.T) {
	library := os.Getenv("PAPERWORKS_TEST_SHEETJS")
	if library == "" {
		t.Fatal("set PAPERWORKS_TEST_SHEETJS to the pinned original reader; missing reader is not a passing or skipped contract")
	}
	library, e := filepath.Abs(library)
	if e != nil {
		t.Fatal(e)
	}
	// Identity of the reader bundled beside the pinned original suite.
	reader, e := os.ReadFile(library)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(reader)) != "cc015130aa8521e7f088f88898eba949ccdcbfb38df0bd129b44b7273c3a6f41" {
		t.Fatal("SheetJS reader does not match pinned original")
	}
	_, source := referenceTraceFixture(t, nil)
	path, items, e := referenceThreadEdit(t, source, CommentSpec{Action: "add", Cell: "B2", Author: "Alex", Text: "Question"})
	if e != nil || len(items) != 1 {
		t.Fatal(items, e)
	}
	path, _, e = referenceThreadEdit(t, path, CommentSpec{Action: "reply", ThreadID: items[0].ID, Author: "Sam", Text: "Answer"})
	if e != nil {
		t.Fatal(e)
	}
	const code = `const x=require(process.argv[1]);const w=x.read(require("fs").readFileSync(process.argv[2]),{type:"buffer"});process.stdout.write(JSON.stringify(w.Sheets.Output.B2.c));`
	cmd := exec.Command("node", "-e", code, library, path)
	raw, e := cmd.Output()
	if e != nil {
		if exit, ok := e.(*exec.ExitError); ok {
			t.Fatalf("SheetJS failed: %v: %s", e, exit.Stderr)
		}
		t.Fatal(e)
	}
	var comments []struct {
		Author string `json:"a"`
		Text   string `json:"t"`
		Thread bool   `json:"T"`
	}
	if e := json.Unmarshal(raw, &comments); e != nil {
		t.Fatal(e)
	}
	var pairs [][2]string
	for _, c := range comments {
		pairs = append(pairs, [2]string{c.Author, c.Text})
	}
	if !reflect.DeepEqual(pairs, [][2]string{{"Alex", "Question"}, {"Sam", "Answer"}}) {
		t.Fatal(pairs)
	}
	for _, c := range comments {
		if !c.Thread {
			t.Fatal("reader did not recognize threaded comment", c)
		}
	}
}
