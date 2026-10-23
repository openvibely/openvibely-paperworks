package pdf

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/primitives"
)

func TestFormAuthoringRejectsMissingGlyphWithoutWritingOutput(t *testing.T) {
	for _, tc := range []struct{ name, field, want string }{
		{"combo option", `"combo_boxes":[{"id":"choice","pos":[10,10],"width":100,"options":["東京"]}]`, "U+6771"},
		{"text value", `"text_fields":[{"id":"name","pos":[10,10],"width":100,"value":"😀"}]`, "U+1F600"},
		{"label", `"text_fields":[{"id":"name","pos":[10,10],"width":100,"label":{"value":"😀","width":30}}]`, "U+1F600"},
		{"unsupported font", `"text_fields":[{"id":"name","pos":[10,10],"width":100,"label":{"value":"Name","width":30,"font":{"name":"UnavailableFont"}}}]`, "unsupported PDF font"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var spec CreateSpec
			if err := json.Unmarshal([]byte(`{"pages":[{`+tc.field+`}]}`), &spec); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "out.pdf")
			err := Create(context.Background(), spec, ".", out)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("output written", err)
			}
		})
	}
}

func TestAuthoredComboSavedValuesAndAppearance(t *testing.T) {
	var spec CreateSpec
	err := json.Unmarshal([]byte(`{"pages":[{"combo_boxes":[{"id":"choice","pos":[100,600],"width":160,"options":["Draft","Approved"],"value":"Draft","default":"Draft","label":{"value":"Status","width":60,"pos":"left"}}]}]}`), &spec)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.pdf")
	if err := Create(ctx, spec, ".", src); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Report(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Issues) != 0 || len(rep.Widgets) != 1 || rep.Fields["choice"] == nil || *rep.Fields["choice"] != "Draft" {
		t.Fatalf("unexpected initial combo report: %+v", rep)
	}
	out := filepath.Join(dir, "filled.pdf")
	if _, err := Fill(ctx, src, out, map[string]string{"choice": "Approved"}, false); err != nil {
		t.Fatal(err)
	}
	rep, err = Report(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Issues) != 0 || rep.Fields["choice"] == nil || *rep.Fields["choice"] != "Approved" {
		t.Fatalf("unexpected filled combo report: %+v", rep)
	}
	after, _ := os.ReadFile(out)
	if sameImage(images(t, before)[0], images(t, after)[0]) {
		t.Fatal("unchanged selected-value appearance")
	}
	flat := filepath.Join(dir, "flat.pdf")
	if _, err := Fill(ctx, out, flat, map[string]string{}, true); err != nil {
		t.Fatal(err)
	}
	rep, err = Report(ctx, flat)
	if err != nil || rep.HasFormTree || len(rep.Widgets) != 0 {
		t.Fatal(rep, err)
	}
	flattened, _ := os.ReadFile(flat)
	if !sameImage(images(t, after)[0], images(t, flattened)[0]) {
		t.Fatal("flatten changed appearance")
	}
	original, _ := os.ReadFile(src)
	if !bytes.Equal(original, before) {
		t.Fatal("source changed")
	}
}

func TestAuthoredUnicodeFormsFillAndFlatten(t *testing.T) {
	ctx := context.Background()
	textField := &primitives.TextField{
		ID: "name", Value: "José", Position: [2]float64{130, 700}, Width: 180, Height: 24,
		Label: &primitives.TextFieldLabel{TextField: primitives.TextField{Value: "Prénom"}, Width: 70, Position: "left"},
	}
	combo := &primitives.ComboBox{
		ID: "country", Position: [2]float64{130, 640}, Width: 180,
		Options: []string{"Côte d’Ivoire", "España"}, Value: "Côte d’Ivoire",
		Label: &primitives.TextFieldLabel{TextField: primitives.TextField{Value: "Pays"}, Width: 70, Position: "left"},
	}
	spec := CreateSpec{Paper: "A4", Pages: []CreatePage{{TextFields: []*primitives.TextField{textField}, ComboBoxes: []*primitives.ComboBox{combo}}}}

	dir := t.TempDir()
	source := filepath.Join(dir, "unicode-form.pdf")
	if err := Create(ctx, spec, ".", source); err != nil {
		t.Fatal(err)
	}
	sourceData, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Report(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Issues) != 0 || len(report.Widgets) != 2 || report.Fields["name"] == nil || *report.Fields["name"] != "José" || report.Fields["country"] == nil || *report.Fields["country"] != "Côte d’Ivoire" {
		t.Fatalf("unexpected initial form report: %+v", report)
	}
	pageText, err := PageText(ctx, sourceData)
	if err != nil {
		t.Fatal(err)
	}
	initialText := strings.Join(pageText, "\n")
	for _, want := range []string{"Prénom"} {
		if !strings.Contains(initialText, want) {
			t.Errorf("generated page text %q does not contain %q", initialText, want)
		}
	}
	presetCtx, _, err := LoadFile(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := flattenForm(ctx, presetCtx); err != nil {
		t.Fatal(err)
	}
	presetData, err := Serialize(ctx, presetCtx)
	if err != nil {
		t.Fatal(err)
	}
	presetText, err := PageText(ctx, presetData)
	if err != nil {
		t.Fatal(err)
	}
	presetPageText := strings.Join(presetText, "\n")
	for _, want := range []string{"Prénom", "Pays", "José", "Côte d’Ivoire"} {
		if !strings.Contains(presetPageText, want) {
			t.Errorf("flattened preset page text %q does not contain %q", presetPageText, want)
		}
	}

	filled := filepath.Join(dir, "filled.pdf")
	if _, err := Fill(ctx, source, filled, map[string]string{"name": "André", "country": "España"}, false); err != nil {
		t.Fatal(err)
	}
	filledReport, err := Report(ctx, filled)
	if err != nil {
		t.Fatal(err)
	}
	if len(filledReport.Issues) != 0 || filledReport.Fields["name"] == nil || *filledReport.Fields["name"] != "André" || filledReport.Fields["country"] == nil || *filledReport.Fields["country"] != "España" {
		t.Fatalf("unexpected filled form report: %+v", filledReport)
	}
	flat := filepath.Join(dir, "flattened.pdf")
	if _, err := Fill(ctx, filled, flat, map[string]string{}, true); err != nil {
		t.Fatal(err)
	}
	flatReport, err := Report(ctx, flat)
	if err != nil || flatReport.HasFormTree || len(flatReport.Widgets) != 0 {
		t.Fatalf("flattened form remains interactive: report=%+v error=%v", flatReport, err)
	}
	flatData, err := os.ReadFile(flat)
	if err != nil {
		t.Fatal(err)
	}
	flatText, err := PageText(ctx, flatData)
	if err != nil {
		t.Fatal(err)
	}
	flattenedText := strings.Join(flatText, "\n")
	for _, want := range []string{"Prénom", "André", "España"} {
		if !strings.Contains(flattenedText, want) {
			t.Errorf("flattened page text %q does not contain %q", flattenedText, want)
		}
	}
	if sameImage(images(t, sourceData)[0], images(t, flatData)[0]) {
		t.Fatal("filled and flattened appearances did not change")
	}

	oneShot := filepath.Join(dir, "filled-and-flattened.pdf")
	if _, err := Fill(ctx, source, oneShot, map[string]string{"name": "André", "country": "España"}, true); err != nil {
		t.Fatal(err)
	}
	oneShotData, err := os.ReadFile(oneShot)
	if err != nil {
		t.Fatal(err)
	}
	oneShotText, err := PageText(ctx, oneShotData)
	if err != nil {
		t.Fatal(err)
	}
	oneShotPageText := strings.Join(oneShotText, "\n")
	for _, want := range []string{"Prénom", "André", "España"} {
		if !strings.Contains(oneShotPageText, want) {
			t.Errorf("one-shot flattened page text %q does not contain %q", oneShotPageText, want)
		}
	}
}
