package pdf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestFillInheritedListChoiceAppearances(t *testing.T) {
	for _, tc := range []struct {
		name             string
		flags            int
		value            any
		values           []string
		labels           []string
		indices          []int
		highlightIndices []int
	}{
		{name: "single", value: "B", values: []string{"B"}, labels: []string{"Beta"}, highlightIndices: []int{1}},
		{name: "multi", flags: ffMultiSelect, value: []string{"Gamma", "A"}, values: []string{"Gamma", "A"}, labels: []string{"Gamma", "Alpha"}, indices: []int{0, 2}, highlightIndices: []int{0, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			source := writeTemp(t, "inherited-list.pdf", inheritedListChoicePDF(tc.flags, ""))
			values := map[string]any{"group.choice": tc.value}

			editable := filepath.Join(t.TempDir(), "editable.pdf")
			if _, err := FillValues(ctx, source, editable, values, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, editable)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			field := fields["group.choice"]
			if tc.flags&ffMultiSelect != 0 {
				got, ok := entryArray(c.XRefTable, field.D["V"])
				if !ok || len(got) != len(tc.values) {
					t.Fatalf("saved /V = %v", field.D["V"])
				}
				for i, item := range got {
					if value := display(c.XRefTable, item); value == nil || *value != tc.values[i] {
						t.Fatalf("saved /V item %d = %v, want %q", i, value, tc.values[i])
					}
				}
				got, ok = entryArray(c.XRefTable, field.D["I"])
				if !ok || len(got) != len(tc.indices) {
					t.Fatalf("saved /I = %v, want %v", field.D["I"], tc.indices)
				}
				for i, index := range tc.indices {
					if got[i] != types.Integer(index) {
						t.Fatalf("saved /I = %v, want %v", got, tc.indices)
					}
				}
			} else {
				if value := display(c.XRefTable, field.D["V"]); value == nil || *value != tc.values[0] {
					t.Fatalf("saved /V = %v, want %q", value, tc.values[0])
				}
				if field.D["I"] != nil {
					t.Fatalf("single selection unexpectedly saved /I: %v", field.D["I"])
				}
			}

			ap := dictOf(c.XRefTable, field.Widgets[0]["AP"]).d
			sd, _ := streamOf(c.XRefTable, ap["N"])
			if sd == nil {
				t.Fatal("missing editable list appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			appearance := string(sd.Content)
			for _, label := range []string{"Alpha", "Beta", "Gamma"} {
				if !strings.Contains(appearance, "("+label+")") {
					t.Errorf("editable appearance is missing inherited display label %q: %s", label, appearance)
				}
			}
			if got := strings.Count(appearance, "0.60 0.76 0.85 rg"); got != len(tc.highlightIndices) {
				t.Errorf("editable selection highlights = %d, want %d: %s", got, len(tc.highlightIndices), appearance)
			}

			flattened := filepath.Join(t.TempDir(), "flattened.pdf")
			if _, err := FillValues(ctx, source, flattened, values, true); err != nil {
				t.Fatal(err)
			}
			flatContext, flatData, err := LoadFile(ctx, flattened)
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := flatContext.XRefTable.Catalog()
			if err != nil {
				t.Fatal(err)
			}
			if catalog["AcroForm"] != nil {
				t.Fatal("flattened output retains AcroForm")
			}
			page, _, err := pageDict(ctx, flatContext, 2)
			if err != nil {
				t.Fatal(err)
			}
			if page["Annots"] != nil {
				t.Fatal("flattened output retains widget annotations")
			}
			texts, err := PageText(ctx, flatData)
			if err != nil {
				t.Fatal(err)
			}
			pageText := strings.Join(texts, "\n")
			for _, label := range tc.labels {
				if !strings.Contains(pageText, label) {
					t.Errorf("flattened page text %q is missing selected label %q", pageText, label)
				}
			}
			flatImages := images(t, flatData)
			for _, index := range tc.highlightIndices {
				y := []int{294, 322, 350}[index]
				pixel := flatImages[1].RGBAAt(180, y)
				if pixel.R >= 200 || pixel.G <= 170 || pixel.B <= 190 {
					t.Errorf("flattened selection highlight at option %d has pixel %+v", index, pixel)
				}
			}
		})
	}
}

func TestInheritedListChoiceEmptyLocalOptionsShadowParent(t *testing.T) {
	ctx := context.Background()
	source := writeTemp(t, "empty-local-options.pdf", inheritedListChoicePDF(ffMultiSelect, "/Opt []"))
	c, err := Load(ctx, mustReadFile(t, source))
	if err != nil {
		t.Fatal(err)
	}
	fields, _, err := fieldTree(c.XRefTable)
	if err != nil {
		t.Fatal(err)
	}
	exports, labels := choiceOptions(c.XRefTable, fields["group.choice"].D)
	if len(exports) != 0 || len(labels) != 0 {
		t.Fatalf("empty local /Opt did not shadow the parent: exports=%v labels=%v", exports, labels)
	}

	output := filepath.Join(t.TempDir(), "strict.pdf")
	_, err = Fill(ctx, source, output, map[string]string{"group.choice": "A"}, false, FillOptions{StrictChoices: true})
	var warning ChoiceWarning
	if !errors.As(err, &warning) || warning.Field != "group.choice" || warning.Value != "A" || len(warning.AllowedChoices) != 0 {
		t.Fatalf("strict validation with empty local /Opt: %v", err)
	}
}

func inheritedListChoicePDF(flags int, localOptions string) []byte {
	objs := formObjs()
	objs[0] = strings.Replace(objs[0], "13 0 R 15 0 R", "13 0 R 29 0 R", 1)
	objs[14] = fmt.Sprintf("<< /FT /Ch /Ff %d /T (choice) /V (A) /Parent 29 0 R %s /DA (/Helv 12 Tf 0 g) /Kids [16 0 R] >>", flags, localOptions)
	objs[15] = strings.Replace(objs[15], "220 220", "220 260", 1)
	objs[27] = strings.Replace(objs[27], "/BBox [0 0 180 20]", "/BBox [0 0 180 60]", 1)
	objs = append(objs, "<< /T (group) /Opt [[(A) (Alpha)] [(B) (Beta)] (Gamma)] /Kids [15 0 R] >>")
	return rawPDF(objs)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
