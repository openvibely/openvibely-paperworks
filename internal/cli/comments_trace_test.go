package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Ports test_cli_comments_and_trace using the original workbook contents,
// including its legacy note. Every operation goes through the CLI dispatcher.
func TestReferenceCLICommentsAndTrace(t *testing.T) {
	dir := t.TempDir()
	source := referenceCLIThreadSource(t, dir)
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	original, e := os.ReadFile(source)
	must(e)
	spec := writeSpec(t, dir, "comment.json", map[string]any{"action": "add", "sheet": "Output", "cell": "B2", "author": "Alex", "text": "Review"})
	output := filepath.Join(dir, "cli.xlsx")
	runCLI(t, "comments", source, "--output", output, "--requirements", spec)
	listing := runCLI(t, "comments", output)
	comments := listing["comments"].([]any)
	if len(comments) != 1 || comments[0].(map[string]any)["text"] != "Review" {
		t.Fatal(listing)
	}
	snapshot, e := os.ReadFile(output)
	must(e)
	after := runCLI(t, "trace", output, "--sheet", "Output", "--cell", "A3")
	if after["complete"] != true {
		t.Fatal(after)
	}
	before := runCLI(t, "trace", source, "--sheet", "Output", "--cell", "A3")
	// Comment authoring must not change dependencies or their saved values.
	for _, key := range []string{"nodes", "edges", "cycles", "unresolved"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			t.Fatalf("comment edit changed trace %s", key)
		}
	}
	reopened, e := excelize.OpenFile(output)
	must(e)
	defer reopened.Close()
	notes, e := reopened.GetComments("Inputs")
	must(e)
	if len(notes) != 1 || notes[0].Text != "Keep this note" || notes[0].Author != "Original" {
		t.Fatal(notes)
	}
	current, e := os.ReadFile(output)
	must(e)
	if !bytes.Equal(snapshot, current) {
		t.Fatal("read-only trace changed commented workbook")
	}
	current, e = os.ReadFile(source)
	must(e)
	if !bytes.Equal(original, current) {
		t.Fatal("source modified")
	}
	// Existing output protection applies to the combined workflow too.
	var stdout, stderr bytes.Buffer
	if Main([]string{"comments", source, "--output", output, "--requirements", spec}, &stdout, &stderr) == 0 {
		t.Fatal("overwrite accepted")
	}
	current, e = os.ReadFile(output)
	must(e)
	if !bytes.Equal(snapshot, current) {
		t.Fatal("overwrite attempt modified file")
	}
}

// New identities must not replace persisted identities or allocate another person
// when the caller explicitly selects an existing author.
func TestReplyAllocationReusesAuthorAndSurvivesRoundTrip(t *testing.T) {
	source := filepath.Join("..", "..", "testdata", "regression", "comment-identity-source.xlsx")
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	original := runCLI(t, "comments", source)["comments"].([]any)
	root := original[0].(map[string]any)
	dir := t.TempDir()
	spec := writeSpec(t, dir, "reply.json", map[string]any{"action": "reply", "sheet": root["sheet"], "thread_id": root["id"], "author": root["author"], "author_id": root["author_id"], "text": "New reply with existing author"})
	filled := filepath.Join(dir, "reply.xlsx")
	items := runCLI(t, "comments", source, "--requirements", spec, "--output", filled)["comments"].([]any)
	if len(items) != len(original)+1 || !reflect.DeepEqual(items[:len(original)], original) {
		t.Fatal("existing identities changed", items)
	}
	added := items[len(original)].(map[string]any)
	for _, c := range original {
		if added["id"] == c.(map[string]any)["id"] {
			t.Fatal("duplicate ID")
		}
	}
	if added["id"] == "" || added["date"] == "" || added["parent_id"] != root["id"] || added["author_id"] != root["author_id"] || added["author"] != root["author"] || added["cell"] != root["cell"] || added["text"] != "New reply with existing author" {
		t.Fatal("bad new reply", added)
	}
	last := filled
	for _, action := range []string{"resolve", "reopen"} {
		req := writeSpec(t, dir, action+".json", map[string]any{"action": action, "sheet": root["sheet"], "thread_id": root["id"]})
		out := filepath.Join(dir, action+".xlsx")
		got := runCLI(t, "comments", last, "--requirements", req, "--output", out)["comments"].([]any)
		if len(got) != len(items) {
			t.Fatal("comment count changed")
		}
		for i := range got {
			for _, key := range []string{"id", "parent_id", "author_id", "author", "date", "text", "sheet", "cell"} {
				if !reflect.DeepEqual(got[i].(map[string]any)[key], items[i].(map[string]any)[key]) {
					t.Fatalf("%s changed %s", action, key)
				}
			}
		}
		if got[0].(map[string]any)["resolved"] != (action == "resolve") {
			t.Fatal("incorrect resolution")
		}
		last = out
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
}

func TestReplyAllocationCreatesPersonAndSurvivesRoundTrip(t *testing.T) {
	source := filepath.Join("..", "..", "testdata", "regression", "comment-identity-source.xlsx")
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	original := runCLI(t, "comments", source)["comments"].([]any)
	root := original[0].(map[string]any)
	dir := t.TempDir()
	spec := writeSpec(t, dir, "reply.json", map[string]any{"action": "reply", "sheet": root["sheet"], "thread_id": root["id"], "author": "Morgan New Reviewer", "text": "Reply from a new person"})
	filled := filepath.Join(dir, "reply.xlsx")
	items := runCLI(t, "comments", source, "--requirements", spec, "--output", filled)["comments"].([]any)
	if len(items) != len(original)+1 || !reflect.DeepEqual(items[:len(original)], original) {
		t.Fatal("existing identities changed", items)
	}
	added := items[len(original)].(map[string]any)
	for _, c := range original {
		if added["author_id"] == c.(map[string]any)["author_id"] {
			t.Fatal("new author reused an existing identity")
		}
		if added["id"] == c.(map[string]any)["id"] {
			t.Fatal("duplicate ID")
		}
	}
	if added["id"] == "" || added["date"] == "" || added["parent_id"] != root["id"] || added["author_id"] == "" || added["author"] != "Morgan New Reviewer" || added["cell"] != root["cell"] || added["text"] != "Reply from a new person" {
		t.Fatal("bad new reply", added)
	}
	last := filled
	for _, action := range []string{"resolve", "reopen"} {
		req := writeSpec(t, dir, action+".json", map[string]any{"action": action, "sheet": root["sheet"], "thread_id": root["id"]})
		out := filepath.Join(dir, action+".xlsx")
		got := runCLI(t, "comments", last, "--requirements", req, "--output", out)["comments"].([]any)
		if len(got) != len(items) {
			t.Fatal("comment count changed")
		}
		for i := range got {
			for _, key := range []string{"id", "parent_id", "author_id", "author", "date", "text", "sheet", "cell"} {
				if !reflect.DeepEqual(got[i].(map[string]any)[key], items[i].(map[string]any)[key]) {
					t.Fatalf("%s changed %s", action, key)
				}
			}
		}
		if got[0].(map[string]any)["resolved"] != (action == "resolve") {
			t.Fatal("incorrect resolution")
		}
		last = out
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source changed", err)
	}
}

func TestReferenceThreadRejectsSameSourceAndDestination(t *testing.T) {
	dir := t.TempDir()
	source := referenceCLIThreadSource(t, dir)
	add := writeSpec(t, dir, "add.json", map[string]any{"action": "add", "sheet": "Output", "cell": "A1", "author": "Alex", "text": "Text"})
	path := filepath.Join(dir, "comments.xlsx")
	runCLI(t, "comments", source, "--output", path, "--requirements", add)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spec := writeSpec(t, dir, "resolve.json", map[string]any{"action": "resolve"})
	var stdout, stderr bytes.Buffer
	if Main([]string{"comments", path, "--output", path, "--requirements", spec}, &stdout, &stderr) == 0 {
		t.Fatal("source overwrite accepted")
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, current) {
		t.Fatal("source changed", err)
	}
}

// Original SpreadsheetThreadsTrace.setUp, shared by both CLI ports.
func referenceCLIThreadSource(t *testing.T, dir string) string {
	t.Helper()
	source := filepath.Join(dir, "source.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(f.SetSheetName("Sheet1", "Inputs"))
	_, e := f.NewSheet("O'Brien")
	must(e)
	_, e = f.NewSheet("Output")
	must(e)
	must(f.SetCellValue("Inputs", "A1", 7))
	must(f.SetCellValue("O'Brien", "A1", 3))
	must(f.AddComment("Inputs", excelize.Comment{Cell: "A1", Author: "Original", Text: "Keep this note"}))
	for c, formula := range map[string]string{"A1": `Inputs!$A$1+'O''Brien'!A1`, "A2": "A1*2", "A3": "SUM(A1:A2)", "B1": "Driver+A1"} {
		must(f.SetCellFormula("Output", c, formula))
	}
	must(f.SetDefinedName(&excelize.DefinedName{Name: "Driver", RefersTo: "Inputs!$A$1"}))
	must(f.SaveAs(source))

	return source
}
