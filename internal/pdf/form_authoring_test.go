package pdf

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormAuthoringTextValidation(t *testing.T) {
	for _, tc := range []struct{ name, field string }{
		{"combo option", `"combo_boxes":[{"options":["東京"]}]`},
		{"combo value", `"combo_boxes":[{"value":"é"}]`},
		{"combo default", `"combo_boxes":[{"default":"é"}]`},
		{"text label", `"text_fields":[{"label":{"value":"é"}}]`},
		{"check label", `"checkboxes":[{"label":{"value":"é"}}]`},
		{"combo label", `"combo_boxes":[{"label":{"value":"é"}}]`},
		{"radio label", `"radio_groups":[{"label":{"value":"é"}}]`},
		{"radio option", `"radio_groups":[{"buttons":{"values":["Yes","é"]}}]`},
		{"button label", `"radio_groups":[{"buttons":{"label":{"value":"é"}}}]`},
		{"label font", `"text_fields":[{"label":{"value":"ASCII","font":{"name":"UnavailableFont"}}}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var spec CreateSpec
			if err := json.Unmarshal([]byte(`{"pages":[{`+tc.field+`}]}`), &spec); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "out.pdf")
			err := Create(context.Background(), spec, ".", out)
			if err == nil || !strings.Contains(err.Error(), "embedded Unicode fonts") {
				t.Fatalf("got %v", err)
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
		t.Fatalf("%+v", rep)
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
		t.Fatalf("%+v", rep)
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
