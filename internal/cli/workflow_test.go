package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

func runCLI(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var out, errout bytes.Buffer
	if code := Main(args, &out, &errout); code != 0 {
		t.Fatalf("%v: exit %d: %s", args, code, errout.String())
	}
	var result map[string]any
	if e := json.Unmarshal(out.Bytes(), &result); e != nil {
		t.Fatal(e, out.String())
	}
	return result
}
func writeSpec(t *testing.T, dir, name string, spec any) string {
	t.Helper()
	p := filepath.Join(dir, name)
	b, e := json.Marshal(spec)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestSpreadsheetCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "revenue.xlsx")
	r := runCLI(t, "create-xlsx", "--spec", "../../testdata/workflows/revenue.json", "--output", original)
	if r["recalculated"] != false {
		t.Fatal("incorrect calculation claim", r)
	}
	before, e := os.ReadFile(original)
	if e != nil {
		t.Fatal(e)
	}
	runCLI(t, "inspect", original)
	comment := writeSpec(t, dir, "comment.json", map[string]any{"action": "add", "sheet": "Revenue", "cell": "B2", "author": "Analyst", "text": "Confirm volume."})
	commented := filepath.Join(dir, "commented.xlsx")
	r = runCLI(t, "comments", original, "--requirements", comment, "--output", commented)
	comments := r["comments"].([]any)
	if len(comments) != 1 {
		t.Fatal(r)
	}
	id := comments[0].(map[string]any)["id"].(string)
	last := commented
	var persisted []any
	for i, action := range []string{"reply", "resolve", "reopen"} {
		spec := map[string]any{"action": action, "sheet": "Revenue", "thread_id": id}
		if action == "reply" {
			spec["author"] = "Reviewer"
			spec["text"] = "Confirmed."
		}
		p := writeSpec(t, dir, action+".json", spec)
		out := filepath.Join(dir, action+".xlsx")
		r = runCLI(t, "comments", last, "--requirements", p, "--output", out)
		items := r["comments"].([]any)
		if len(items) != 2 {
			t.Fatal(r)
		}
		if items[0].(map[string]any)["resolved"] != (i == 1) {
			t.Fatal("wrong resolution", r)
		}
		if i == 0 {
			persisted = items
		} else {
			for j, item := range items {
				got, want := item.(map[string]any), persisted[j].(map[string]any)
				for _, key := range []string{"id", "parent_id", "author_id", "date", "author", "text", "sheet", "cell"} {
					if !reflect.DeepEqual(got[key], want[key]) {
						t.Fatalf("%s changed comment %d %s: %v != %v", action, j, key, got[key], want[key])
					}
				}
			}
		}
		last = out
	}
	inputs := writeSpec(t, dir, "inputs.json", map[string]any{"sheet": "Revenue", "cells": map[string]any{"B2": 5}})
	edited := filepath.Join(dir, "edited.xlsx")
	runCLI(t, "workbook-inputs", last, "--requirements", inputs, "--output", edited)
	f, e := excelize.OpenFile(edited)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	v, _ := f.GetCellValue("Revenue", "B2")
	formula, _ := f.GetCellFormula("Revenue", "D2")
	if v != "5" || formula != "B2*C2" {
		t.Fatal(v, formula)
	}
	p, e := opc.Open(edited)
	if e != nil {
		t.Fatal(e)
	}
	threads, e := office.WorkbookComments(p, office.CommentSpec{})
	if e != nil || len(threads) != 2 {
		t.Fatal(threads, e)
	}
	source, e := opc.Open(last)
	if e != nil {
		t.Fatal(e)
	}
	for _, part := range source.Parts {
		if part.Name == "xl/worksheets/sheet1.xml" {
			continue
		}
		var got []byte
		for _, candidate := range p.Parts {
			if candidate.Name == part.Name {
				got = candidate.Data
				break
			}
		}
		if !bytes.Equal(part.Data, got) {
			t.Fatalf("unrelated part changed: %s", part.Name)
		}
	}
	after, _ := os.ReadFile(original)
	if !bytes.Equal(before, after) {
		t.Fatal("original changed")
	}
	listed := runCLI(t, "comments", edited)
	if !reflect.DeepEqual(listed["comments"], persisted) {
		t.Fatal(listed)
	}
}

func TestTraceCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "model.xlsx")
	runCLI(t, "create-xlsx", "--spec", "../../testdata/workflows/revenue.json", "--output", output)
	result := runCLI(t, "trace", output, "--sheet", "Revenue", "--cell", "D4")
	if result["complete"] != true || len(result["nodes"].(map[string]any)) != 7 {
		t.Fatal(result)
	}
}

func TestFlowPDFCLIPageCount(t *testing.T) {
	output := filepath.Join(t.TempDir(), "flow.pdf")
	result := runCLI(t, "create-pdf", "--spec", "../../testdata/workflows/revenue-flow.json", "--output", output)
	if result["pages"] != float64(1) {
		t.Fatalf("flow page count: %+v", result)
	}
	report := runCLI(t, "inspect", output)
	if report["pages"] != float64(1) {
		t.Fatalf("saved page count: %+v", report)
	}
}
