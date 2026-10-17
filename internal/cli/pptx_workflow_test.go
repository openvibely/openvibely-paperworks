package cli

import (
	"bytes"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"path/filepath"
	"testing"
)

func TestPresentationCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "deck.pptx")
	runCLI(t, "create-pptx", "--spec", "../../testdata/workflows/revenue-deck.json", "--output", output)
	runCLI(t, "inspect", output)
	verified := runCLI(t, "verify-slides", output, "--requirements", "../../testdata/workflows/revenue-deck-checks.json")
	if verified["requirements_passed"] != true {
		t.Fatal("successful verification must report requirements_passed", verified)
	}
	edited := filepath.Join(dir, "edited.pptx")
	runCLI(t, "slide-edit", output, "--spec", "../../testdata/workflows/revenue-deck-edit.json", "--output", edited)
	runCLI(t, "verify-slides", edited, "--requirements", "../../testdata/workflows/revenue-deck-checks.json")
	p, err := opc.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if filepath.Ext(part.Name) == ".xml" || filepath.Ext(part.Name) == ".rels" {
			if _, err := p.XML(part.Name); err != nil {
				t.Fatal(err)
			}
		}
	}
	pres, _ := p.XML("ppt/presentation.xml")
	if len(office.All(pres.Root(), office.NSP, "notesMasterId")) != 1 {
		t.Fatal("missing notes master registration")
	}
	for i := 1; i <= 8; i++ {
		name := fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i)
		d, e := p.XML(name)
		if e != nil {
			t.Fatal(e)
		}
		text := office.All(d.Root(), office.NSA, "t")
		if len(text) == 0 || text[0].Text() == "" {
			t.Fatal("missing notes", i)
		}
		rels, e := p.Rels(name)
		if e != nil || len(rels) != 2 {
			t.Fatal(rels, e)
		}
	}
	// Source workbook remains editable and contains the exact projections.
	f, e := excelize.OpenReader(bytes.NewReader(p.Get("ppt/embeddings/Microsoft_Excel_Sheet2.xlsx")))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	for cell, want := range map[string]string{"A4": "FY2029", "B4": "172.8", "C4": "88.2", "D4": "261"} {
		got, e := f.GetCellValue("Sheet1", cell)
		if e != nil || got != want {
			t.Fatal(cell, got, want, e)
		}
	}
	// Repairing already-correct notes is idempotent and preserves every unrelated part.
	repaired := filepath.Join(dir, "repaired.pptx")
	runCLI(t, "repair-notes", output, "--output", repaired)
	q, e := opc.Open(repaired)
	if e != nil {
		t.Fatal(e)
	}
	for _, part := range p.Parts {
		if !bytes.Equal(part.Data, q.Get(part.Name)) {
			t.Fatal("unexpected repair change", part.Name)
		}
	}
}

func TestPresentationCommentsCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "deck.pptx")
	runCLI(t, "create-pptx", "--spec", "../../testdata/workflows/revenue-deck.json", "--output", source)
	for _, spec := range []map[string]any{{"action": "add", "slide": 1, "author": "Reviewer", "text": "Review the baseline."}, {"action": "add", "slide": 2, "shape_id": 4, "author": "Reviewer", "text": "Check the table."}, {"action": "add", "slide": 2, "shape_id": 2, "text_match": "sample revenue", "author": "Reviewer", "text": "Keep sample qualification."}} {
		p := writeSpec(t, dir, fmt.Sprintf("comment-%d.json", len(spec)), spec)
		out := filepath.Join(dir, fmt.Sprintf("commented-%d.pptx", len(spec)))
		r := runCLI(t, "slide-comments", source, "--requirements", p, "--output", out)
		if r["native_powerpoint_ui_verified"] != false {
			t.Fatal("false UI claim")
		}
		runCLI(t, "slide-comments", out)
		source = out
	}
	r := runCLI(t, "slide-comments", source)
	if len(r["comments"].([]any)) != 3 {
		t.Fatal(r)
	}
}
