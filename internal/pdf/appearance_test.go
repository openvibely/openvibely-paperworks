package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestFillPreservesWidgetDecoration(t *testing.T) {
	for _, field := range []string{"name", "choice"} {
		t.Run(field, func(t *testing.T) {
			objs := formObjs()
			index := 13
			value := "Ada"
			if field == "choice" {
				index, value = 15, "Beta"
			}
			objs[index] = strings.Replace(objs[index], "/F 4", "/F 4 /MK << /BG [0.8 0.843 1] /BC [0.1 0.1 0.1] >> /BS << /W 1 /S /S >>", 1)
			before := rawPDF(objs)
			source := writeTemp(t, "decorated.pdf", before)
			out := filepath.Join(t.TempDir(), "filled.pdf")
			ctx := context.Background()
			if _, err := Fill(ctx, source, out, map[string]string{field: value}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, out)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			ap := dictOf(c.XRefTable, fields[field].Widgets[0]["AP"]).d
			sd, _ := streamOf(c.XRefTable, ap["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"0.8 0.843 1 rg", "0.1 0.1 0.1 RG", "1 w", "1 1 178 18 re s", "(" + value + ") Tj"} {
				if !strings.Contains(string(sd.Content), want) {
					t.Fatalf("missing %q in %s", want, sd.Content)
				}
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			img := images(t, data)[1]
			y := 170
			if field == "choice" {
				y = 370
			}
			bg := img.RGBAAt(400, y)
			if bg.R != 204 || bg.G != 215 || bg.B != 255 {
				t.Fatalf("background color lost: %v", bg)
			}
			border := img.RGBAAt(82, y)
			if border.R > 40 || border.G > 40 || border.B > 40 {
				t.Fatalf("border lost: %v", border)
			}
			after, _ := os.ReadFile(source)
			if string(after) != string(before) {
				t.Fatal("source changed")
			}
		})
	}
}

// Exercise different field heights and font sizes so baseline centering cannot
// accidentally be a fixture-specific offset. Inspect the saved PDF, not an
// intermediate appearance, and check flattening preserves its pixels.
func TestSingleLineStandardFontLayout(t *testing.T) {
	for _, tc := range []struct {
		name         string
		font         string
		size, height int
		baseline     string
	}{
		{"small", "Helvetica", 10, 20, "6.41"}, {"reference", "Helvetica", 12, 36, "13.692"}, {"large", "Helvetica", 18, 60, "23.538"},
		{"helvetica-bold-small", "Helvetica-Bold", 10, 20, "6.41"},
		{"helvetica-bold-reference", "Helvetica-Bold", 12, 36, "13.692"},
		{"helvetica-bold-large", "Helvetica-Bold", 18, 60, "23.538"},
		{"helvetica-boldoblique-small", "Helvetica-BoldOblique", 10, 20, "6.41"},
		{"helvetica-boldoblique-reference", "Helvetica-BoldOblique", 12, 36, "13.692"},
		{"helvetica-boldoblique-large", "Helvetica-BoldOblique", 18, 60, "23.538"},
		{"helvetica-oblique-small", "Helvetica-Oblique", 10, 20, "6.41"},
		{"helvetica-oblique-reference", "Helvetica-Oblique", 12, 36, "13.692"},
		{"helvetica-oblique-large", "Helvetica-Oblique", 18, 60, "23.538"},
		{"courier-bold", "Courier-Bold", 12, 36, "14.226"},
		{"courier-oblique", "Courier-Oblique", 10, 20, "6.855"},
		{"courier-bold-oblique", "Courier-BoldOblique", 18, 60, "24.339"},
		{"courier-small", "Courier", 10, 20, "6.855"}, {"courier-reference", "Courier", 12, 36, "14.226"}, {"courier-large", "Courier", 18, 60, "24.339"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objs := formObjs()
			objs[4] = strings.ReplaceAll(objs[4], "/Helvetica", "/"+tc.font)
			objs[12] = strings.Replace(objs[12], "/Helv 12 Tf", fmt.Sprintf("/Helv %d Tf", tc.size), 1)
			objs[13] = strings.Replace(objs[13], "220 320", fmt.Sprintf("220 %d", 300+tc.height), 1)
			objs[26] = strings.Replace(objs[26], "/BBox [0 0 180 20]", fmt.Sprintf("/BBox [0 0 180 %d]", tc.height), 1)
			source := writeTemp(t, "layout.pdf", rawPDF(objs))
			filled := filepath.Join(t.TempDir(), "filled.pdf")
			flat := filepath.Join(t.TempDir(), "flat.pdf")
			value := "A very long value that extends beyond the right edge of this text field"
			ctx := context.Background()
			if _, err := Fill(ctx, source, filled, map[string]string{"name": value, "choice": "Alpha"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, filled)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			ap := dictOf(c.XRefTable, fields["name"].Widgets[0]["AP"]).d
			sd, _ := streamOf(c.XRefTable, ap["N"])
			if sd == nil {
				t.Fatal("missing saved appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{fmt.Sprintf("2 0 174 %d re W BT", tc.height), "2 " + tc.baseline + " Td (" + value + ") Tj"} {
				if !strings.Contains(string(sd.Content), want) {
					t.Fatalf("missing %q in %s", want, sd.Content)
				}
			}
			if _, err := Fill(ctx, filled, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(filled)
			b, _ := os.ReadFile(flat)
			// Only page two contains the edited text and combo fields.
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("flatten changed saved layout")
			}
		})
	}
}

// Coordinates independently taken from the same-input reference appearances.
func TestAlignedSingleLineLayout(t *testing.T) {
	for _, tc := range []struct {
		name string
		q    int
		x    string
	}{
		{"center", 1, "79.326"}, {"right", 2, "156.652"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objs := formObjs()
			objs[12] = strings.Replace(objs[12], "/FT /Tx", fmt.Sprintf("/Q %d /FT /Tx", tc.q), 1)
			source := writeTemp(t, "aligned.pdf", rawPDF(objs))
			out := filepath.Join(t.TempDir(), "filled.pdf")
			flat := filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := Fill(ctx, source, out, map[string]string{"name": "Ada", "choice": "Alpha"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, out)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			ap := dictOf(c.XRefTable, fields["name"].Widgets[0]["AP"]).d
			sd, _ := streamOf(c.XRefTable, ap["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"2 0 174 20 re W BT", tc.x + " 5.692 Td (Ada) Tj"} {
				if !strings.Contains(string(sd.Content), want) {
					t.Fatalf("missing %q in %s", want, sd.Content)
				}
			}
			if _, err := Fill(ctx, out, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(out)
			b, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("flatten changed alignment")
			}
		})
	}
}

func TestMultilineCoreFontLayout(t *testing.T) {
	for _, font := range []string{"Times-BoldItalic", "Times-Italic", "Times-Bold", "Times-Roman", "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique", "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique"} {
		for _, value := range []string{"Ada\nLovelace", "Ada\vLovelace", "Ada\fLovelace", "Ada\r\nLovelace", "A long sentence with enough words to wrap across several lines inside the field.", "A (literal) \\ path\nSecond line"} {
			t.Run(font+"/"+value, func(t *testing.T) {
				objs := formObjs()
				for i := range objs {
					objs[i] = strings.ReplaceAll(objs[i], "/BaseFont /Helvetica", "/BaseFont /"+font)
				}
				objs[12] = strings.Replace(objs[12], "/FT /Tx", "/FT /Tx /Ff 4096", 1)
				objs[13] = strings.Replace(objs[13], "220 320", "220 360", 1)
				objs[26] = strings.Replace(objs[26], "/BBox [0 0 180 20]", "/BBox [0 0 180 60]", 1)
				before := rawPDF(objs)
				source := writeTemp(t, "multiline.pdf", before)
				filled := filepath.Join(t.TempDir(), "filled.pdf")
				flat := filepath.Join(t.TempDir(), "flat.pdf")
				ctx := context.Background()
				if _, err := Fill(ctx, source, filled, map[string]string{"name": value, "choice": "Alpha"}, false); err != nil {
					t.Fatal(err)
				}
				c, _, err := LoadFile(ctx, filled)
				if err != nil {
					t.Fatal(err)
				}
				fields, _, err := fieldTree(c.XRefTable)
				if err != nil {
					t.Fatal(err)
				}
				if stored, ok := textOf(entry(c.XRefTable, fields["name"].D, "V")); !ok || stored != value {
					t.Fatalf("stored value changed: %q, want %q", stored, value)
				}
				ap := dictOf(c.XRefTable, fields["name"].Widgets[0]["AP"]).d
				sd, _ := streamOf(c.XRefTable, ap["N"])
				if sd == nil {
					t.Fatal("missing appearance")
				}
				if err := sd.Decode(); err != nil {
					t.Fatal(err)
				}
				baseline, leading := "2 48.828 Td", "0 -13.872 Td"
				if font == "Times-BoldItalic" {
					baseline, leading = "2 48.948 Td", "0 -13.668 Td"
				}
				if font == "Times-Italic" {
					baseline, leading = "2 49.404 Td", "0 -13.2 Td"
				}
				if font == "Times-Bold" {
					baseline, leading = "2 48.78 Td", "0 -13.836 Td"
				}
				if font == "Times-Roman" {
					baseline, leading = "2 49.224 Td", "0 -13.392 Td"
				}
				if font == "Helvetica-Bold" || font == "Helvetica-BoldOblique" {
					baseline, leading = "2 48.456 Td", "0 -14.28 Td"
				}
				if font == "Courier" || font == "Courier-Oblique" {
					baseline, leading = "2 50.34 Td", "0 -12.66 Td"
				}
				if font == "Courier-Bold" || font == "Courier-BoldOblique" {
					baseline, leading = "2 50.388 Td", "0 -12.612 Td"
				}
				for _, want := range []string{"2 0 174 60 re W BT", baseline} {
					if !strings.Contains(string(sd.Content), want) {
						t.Fatalf("missing %q in %s", want, sd.Content)
					}
				}
				lines := strings.Count(value, "\n") + strings.Count(value, "\v") + strings.Count(value, "\f") + 1
				if strings.Count(string(sd.Content), " Tj") != lines {
					t.Fatalf("unexpected wrapping: %s", sd.Content)
				}
				if lines > 1 && !strings.Contains(string(sd.Content), leading) {
					t.Fatalf("wrong leading: %s", sd.Content)
				}
				if strings.Contains(value, "(literal)") {
					for _, want := range []string{`A \(literal\) \\ path`, `(Second line) Tj`} {
						if !strings.Contains(string(sd.Content), want) {
							t.Fatalf("missing escaped multiline text %q: %s", want, sd.Content)
						}
					}
				}
				if _, err := Fill(ctx, filled, flat, nil, true); err != nil {
					t.Fatal(err)
				}
				a, _ := os.ReadFile(filled)
				b, _ := os.ReadFile(flat)
				if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
					t.Fatal("flatten changed multiline pixels")
				}
				after, err := os.ReadFile(source)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("source changed", err)
				}
			})
		}
	}
}

func TestMultilineQuadding(t *testing.T) {
	for _, tc := range []struct{ q, first, second string }{
		{"1", "79.326 48.828 Td", "-13.338 -13.872 Td"},
		{"2", "156.652 48.828 Td", "-26.676 -13.872 Td"},
	} {
		t.Run(tc.q, func(t *testing.T) {
			objs := formObjs()
			objs[12] = strings.Replace(objs[12], "/FT /Tx", "/FT /Tx /Ff 4096 /Q "+tc.q, 1)
			objs[13] = strings.Replace(objs[13], "220 320", "220 360", 1)
			objs[26] = strings.Replace(objs[26], "/BBox [0 0 180 20]", "/BBox [0 0 180 60]", 1)
			before := rawPDF(objs)
			source := writeTemp(t, "aligned.pdf", before)
			filled := filepath.Join(t.TempDir(), "filled.pdf")
			flat := filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := Fill(ctx, source, filled, map[string]string{"name": "Ada\nLovelace", "choice": "Alpha"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, filled)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			ap := dictOf(c.XRefTable, fields["name"].Widgets[0]["AP"]).d
			sd, _ := streamOf(c.XRefTable, ap["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tc.first, tc.second} {
				if !strings.Contains(string(sd.Content), want) {
					t.Fatalf("missing %q in %s", want, sd.Content)
				}
			}
			if _, err := Fill(ctx, filled, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(filled)
			b, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("flatten changed aligned multiline pixels")
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestScalarListLayoutAndFlatten(t *testing.T) {
	for _, font := range []string{"Times-BoldItalic", "Times-Italic", "Times-Bold", "Times-Roman", "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique", "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique"} {
		t.Run(font, func(t *testing.T) { testScalarListLayoutAndFlatten(t, font, 0, false) })
	}
}

func TestListTopIndexLayoutAndFlatten(t *testing.T) {
	testScalarListLayoutAndFlatten(t, "Helvetica", 1, false)
}

func TestMultiSelectScalarLayoutAndFlatten(t *testing.T) {
	testScalarListLayoutAndFlatten(t, "Helvetica", 0, true)
}

func testScalarListLayoutAndFlatten(t *testing.T, font string, topIndex int, multiSelect bool) {
	baseline, leading := "25.692", "13.872"
	if font == "Times-BoldItalic" {
		baseline, leading = "25.902", "13.668"
	}
	if font == "Times-Italic" {
		baseline, leading = "25.902", "13.2"
	}
	if font == "Times-Bold" {
		baseline, leading = "25.902", "13.836"
	}
	if font == "Times-Roman" {
		baseline, leading = "25.902", "13.392"
	}
	if font == "Helvetica-Bold" || font == "Helvetica-BoldOblique" {
		leading = "14.28"
	}
	if font == "Courier" || font == "Courier-Oblique" {
		baseline, leading = "26.226", "12.66"
	}
	if font == "Courier-Bold" || font == "Courier-BoldOblique" {
		baseline, leading = "26.226", "12.612"
	}
	for _, value := range []string{"Alpha", "Beta", `A (B) \ C`} {
		t.Run(value, func(t *testing.T) {
			objs := formObjs()
			for i, obj := range objs {
				objs[i] = strings.ReplaceAll(obj, "/BaseFont /Helvetica", "/BaseFont /"+font)
			}
			flags := 0
			if multiSelect {
				flags = 1 << 21
			}
			objs[14] = strings.Replace(objs[14], "/Ff 131072", fmt.Sprintf("/Ff %d /TI %d", flags, topIndex), 1)
			objs[14] = strings.Replace(objs[14], "(Beta)]", `(Beta) (A \(B\) \\ C)]`, 1)
			objs[15] = strings.Replace(objs[15], "220 220", "220 260", 1)
			objs[27] = strings.Replace(objs[27], "/BBox [0 0 180 20]", "/BBox [0 0 180 60]", 1)
			before := rawPDF(objs)
			source := writeTemp(t, "list.pdf", before)
			filled, flat := filepath.Join(t.TempDir(), "filled.pdf"), filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := Fill(ctx, source, filled, map[string]string{"choice": value, "name": "Ada"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, filled)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			if got := intOf(c.XRefTable, fields["choice"].D, "Ff"); got != flags {
				t.Fatalf("flags = %d, want %d", got, flags)
			}
			if _, ok := fields["choice"].D["I"]; ok {
				t.Fatal("scalar fill retained indices")
			}
			if got := intOf(c.XRefTable, fields["choice"].D, "TI"); got != topIndex {
				t.Fatalf("top index = %d, want %d", got, topIndex)
			}
			ap := dictOf(c.XRefTable, fields["choice"].Widgets[0]["AP"]).d
			sd, _ := streamOf(c.XRefTable, ap["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"2 0 174 60 re W BT", "2 " + baseline + " Td (Alpha)", "0 -" + leading + " Td (Beta)", "0 -" + leading + ` Td (A \(B\) \\ C)`, "0.5 0.5 0.5 rg s"} {
				if !strings.Contains(string(sd.Content), want) {
					t.Fatalf("missing %q in %s", want, sd.Content)
				}
			}
			if _, err := Fill(ctx, filled, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(filled)
			b, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("flatten changed list appearance")
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
		})
	}
}

func TestArrayListSelectionAndFlatten(t *testing.T) {
	for _, selected := range [][]string{{"Alpha", "Beta"}, {"Beta", "Alpha"}, {"AlphaBeta"}, {"Alpha", "AlphaBeta"}, {}} {
		t.Run(fmt.Sprint(selected), func(t *testing.T) {
			objs := formObjs()
			objs[14] = strings.Replace(objs[14], "/Ff 131072", "/Ff 2097152", 1)
			objs[15] = strings.Replace(objs[15], "220 220", "220 260", 1)
			objs[27] = strings.Replace(objs[27], "/BBox [0 0 180 20]", "/BBox [0 0 180 60]", 1)
			objs[14] = strings.Replace(objs[14], "/Opt [(Alpha) (Beta)]", "/Opt [(Alpha) (Beta) (AlphaBeta)]", 1)
			before := rawPDF(objs)
			source := writeTemp(t, "list.pdf", before)
			filled, flat := filepath.Join(t.TempDir(), "filled.pdf"), filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := FillValues(ctx, source, filled, map[string]any{"choice": selected, "name": "Ada"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, filled)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			f := fields["choice"]
			a, ok := entryArray(c.XRefTable, f.D["V"])
			if !ok || len(a) != len(selected) {
				t.Fatalf("selection = %v", f.D["V"])
			}
			for i, item := range a {
				got := display(c.XRefTable, item)
				if got == nil || *got != selected[i] {
					t.Fatalf("selection item %d = %v", i, got)
				}
			}
			wantIndices := types.Array{}
			for i, option := range []string{"Alpha", "Beta", "AlphaBeta"} {
				for _, item := range selected {
					if item == option {
						wantIndices = append(wantIndices, types.Integer(i))
						break
					}
				}
			}
			gotIndices, hasIndices := entryArray(c.XRefTable, f.D["I"])
			if len(wantIndices) == 0 {
				if hasIndices && len(gotIndices) != 0 {
					t.Fatalf("unexpected selection indices: %v", gotIndices)
				}
			} else if !hasIndices || len(gotIndices) != len(wantIndices) {
				t.Fatalf("selection indices = %v, want %v", f.D["I"], wantIndices)
			} else {
				for i := range wantIndices {
					if gotIndices[i] != wantIndices[i] {
						t.Fatalf("selection indices = %v, want %v", gotIndices, wantIndices)
					}
				}
			}
			ap := dictOf(c.XRefTable, f.Widgets[0]["AP"]).d
			sd, _ := streamOf(c.XRefTable, ap["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			wantHighlights := 0
			joined := strings.Join(selected, "")
			for _, label := range []string{"Alpha", "Beta", "AlphaBeta"} {
				if strings.Contains(joined, label) {
					wantHighlights++
				}
			}
			if got := strings.Count(string(sd.Content), "0.5 0.5 0.5 rg s"); got != wantHighlights {
				t.Fatalf("reference selection outlines = %d, want %d", got, wantHighlights)
			}
			if _, err := Fill(ctx, filled, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			aBytes, _ := os.ReadFile(filled)
			bBytes, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, aBytes)[1].Pix, images(t, bBytes)[1].Pix) {
				t.Fatal("flatten changed multiple selections")
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
		})
	}
	source := writeTemp(t, "form.pdf", rawPDF(formObjs()))
	for _, values := range []map[string]any{{"choice": []string{"Alpha"}}, {"name": []string{"Ada"}}, {"choice": []any{42}}, {"name": nil}} {
		output := filepath.Join(t.TempDir(), "invalid.pdf")
		if _, err := FillValues(context.Background(), source, output, values, false); err == nil {
			t.Fatalf("accepted invalid values: %v", values)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("invalid request created output")
		}
	}
}

// Saved appearances follow the reference's solid redraw for dashed widgets;
// the original dash specification must survive for subsequent viewer edits.
func TestDashedWidgetSavedAppearance(t *testing.T) {
	objs := formObjs()
	objs[14] = strings.Replace(objs[14], "/Ff 131072", "/Ff 2097152", 1)
	for _, index := range []int{13, 15} {
		objs[index] = strings.Replace(objs[index], "/F 4", "/F 4 /MK << /BG [0.8 0.843 1] /BC [0.1 0.1 0.1] >> /BS << /W 1 /S /D /D [3] >>", 1)
	}
	before := rawPDF(objs)
	source := writeTemp(t, "dashed.pdf", before)
	filled, flat := filepath.Join(t.TempDir(), "filled.pdf"), filepath.Join(t.TempDir(), "flat.pdf")
	ctx := context.Background()
	if _, err := FillValues(ctx, source, filled, map[string]any{"name": "Ada", "choice": []string{"Alpha", "Beta"}}, false); err != nil {
		t.Fatal(err)
	}
	c, _, err := LoadFile(ctx, filled)
	if err != nil {
		t.Fatal(err)
	}
	fields, _, err := fieldTree(c.XRefTable)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"name", "choice"} {
		w := fields[key].Widgets[0]
		bs := dictOf(c.XRefTable, w["BS"]).d
		dash, ok := numbers(c.XRefTable, bs["D"])
		if nameOf(c.XRefTable, bs, "S") != "D" || !ok || len(dash) != 1 || dash[0] != 3 {
			t.Fatalf("dash specification lost: %v", bs)
		}
		sd, _ := streamOf(c.XRefTable, dictOf(c.XRefTable, w["AP"]).d["N"])
		if sd == nil {
			t.Fatal("missing appearance")
		}
		if err := sd.Decode(); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"0.8 0.843 1 rg", "1 1 178 18 re s", "2 5.692 Td"} {
			if !strings.Contains(string(sd.Content), want) {
				t.Fatalf("%s missing %q: %s", key, want, sd.Content)
			}
		}
	}
	if _, err := Fill(ctx, filled, flat, nil, true); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filled)
	b, _ := os.ReadFile(flat)
	if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
		t.Fatal("flatten changed saved appearance")
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
}

func TestBorderlessListClippingAndFlatten(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			objs := formObjs()
			objs[14] = strings.Replace(objs[14], "/Ff 131072", "/Ff 2097152", 1)
			decoration := "/MK << /BG [0.8 0.843 1] >>"
			if explicit {
				decoration += " /BS << /W 0 /S /S >>"
			}
			objs[15] = strings.Replace(objs[15], "/F 4", "/F 4 "+decoration, 1)
			before := rawPDF(objs)
			source := writeTemp(t, "borderless.pdf", before)
			filled, flat := filepath.Join(t.TempDir(), "filled.pdf"), filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := FillValues(ctx, source, filled, map[string]any{"name": "Ada", "choice": []string{"Alpha", "Beta"}}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, filled)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			w := fields["choice"].Widgets[0]
			if (w["BS"] != nil) != explicit {
				t.Fatal("border dictionary presence changed")
			}
			sd, _ := streamOf(c.XRefTable, dictOf(c.XRefTable, w["AP"]).d["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(sd.Content), "2 0 174 20 re W BT") {
				t.Fatalf("wrong borderless clip: %s", sd.Content)
			}
			if _, err := Fill(ctx, filled, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(filled)
			b, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("flatten changed saved appearance")
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
		})
	}
}

func TestCombGlyphCentersAndStoredOverflow(t *testing.T) {
	for _, value := range []string{"Wi", "Ab12WxyzMORE", `W(\\)`, ""} {
		t.Run(value, func(t *testing.T) {
			objs := formObjs()
			objs[12] = strings.Replace(objs[12], "/FT /Tx", "/FT /Tx /Ff 16777216 /MaxLen 8", 1)
			before := rawPDF(objs)
			source := writeTemp(t, "comb.pdf", before)
			out := filepath.Join(t.TempDir(), "filled.pdf")
			flat := filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := Fill(ctx, source, out, map[string]string{"name": value, "choice": "Alpha"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, out)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			f := fields["name"]
			if got, ok := textOf(entry(c.XRefTable, f.D, "V")); !ok || got != value {
				t.Fatalf("stored value changed: %q", got)
			}
			if intOf(c.XRefTable, f.D, "MaxLen") != 8 || f.Ff&(1<<24) == 0 {
				t.Fatal("comb flags lost")
			}
			sd, _ := streamOf(c.XRefTable, dictOf(c.XRefTable, f.Widgets[0]["AP"]).d["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			stream := string(sd.Content)
			if value == "Wi" {
				for _, want := range []string{"5.586 5.692 Td (W) Tj", "26.832 0 Td (i) Tj"} {
					if !strings.Contains(stream, want) {
						t.Fatalf("missing %q: %s", want, stream)
					}
				}
			}
			if value == "Ab12WxyzMORE" && (strings.Count(stream, ") Tj") != 8 || strings.Contains(stream, "(M) Tj")) {
				t.Fatalf("overflow not clipped to cells: %s", stream)
			}
			if value == `W(\\)` {
				for _, want := range []string{`(\() Tj`, `(\\) Tj`, `(\)) Tj`} {
					if !strings.Contains(stream, want) {
						t.Fatalf("missing escaped glyph %q: %s", want, stream)
					}
				}
				if strings.Count(stream, `(\\) Tj`) != 2 {
					t.Fatalf("backslash glyphs were not escaped individually: %s", stream)
				}
			}
			if _, err := Fill(ctx, out, flat, map[string]string{}, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(out)
			b, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("later flatten changed comb pixels")
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
		})
	}
}

func TestRotatedTextSavedAppearance(t *testing.T) {
	for _, rotation := range []int{90, 180, 270} {
		t.Run(fmt.Sprint(rotation), func(t *testing.T) {
			objs := formObjs()
			objs[13] = strings.Replace(objs[13], "/F 4", fmt.Sprintf("/F 4 /FT /Tx /MK << /R %d /BG [0.8 0.843 1] /BC [0.1 0.1 0.1] >> /BS << /W 1 /S /S >>", rotation), 1)
			before := rawPDF(objs)
			source := writeTemp(t, "rotation.pdf", before)
			out, flat := filepath.Join(t.TempDir(), "filled.pdf"), filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := Fill(ctx, source, out, map[string]string{"name": "Ada", "choice": "Alpha"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, out)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			w := fields["name"].Widgets[0]
			if intOf(c.XRefTable, dictOf(c.XRefTable, w["MK"]).d, "R") != rotation {
				t.Fatal("rotation lost")
			}
			sd, _ := streamOf(c.XRefTable, dictOf(c.XRefTable, w["AP"]).d["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"0.8 0.843 1 rg", "1 1 178 18 re s", "2 5.692 Td (Ada)"} {
				if !strings.Contains(string(sd.Content), want) {
					t.Fatalf("missing %q: %s", want, sd.Content)
				}
			}
			if _, err := Fill(ctx, out, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(out)
			b, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("flatten changed saved pixels")
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
		})
	}
}

func TestAutomaticSingleLineHelveticaSize(t *testing.T) {
	for _, tc := range []struct{ value, size, baseline string }{
		{"Wi", "29.4", "7.445"},
		{"A long sentence that exceeds the width of this field.", "7.7", "15.236"},
		{strings.Repeat("W", 120), "4", "16.564"},
	} {
		t.Run(tc.size, func(t *testing.T) {
			objs := formObjs()
			objs[12] = strings.Replace(objs[12], "/Helv 12 Tf", "/Helv 0 Tf", 1)
			objs[13] = strings.Replace(objs[13], "220 320", "220 336", 1)
			objs[26] = strings.Replace(objs[26], "/BBox [0 0 180 20]", "/BBox [0 0 180 36]", 1)
			objs[13] = strings.Replace(objs[13], "/F 4", "/F 4 /BS << /W 1 /S /S >>", 1)
			before := rawPDF(objs)
			source := writeTemp(t, "auto.pdf", before)
			out, flat := filepath.Join(t.TempDir(), "filled.pdf"), filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := Fill(ctx, source, out, map[string]string{"name": tc.value, "choice": "Alpha"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, out)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			f := fields["name"]
			if got, _ := textOf(entry(c.XRefTable, f.D, "V")); got != tc.value {
				t.Fatalf("stored value changed: %q", got)
			}
			if got, _ := textOf(entry(c.XRefTable, f.D, "DA")); !strings.Contains(got, "/Helv 0 Tf") {
				t.Fatalf("auto size request changed: %q", got)
			}
			sd, _ := streamOf(c.XRefTable, dictOf(c.XRefTable, f.Widgets[0]["AP"]).d["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"/Helv " + tc.size + " Tf", "2 " + tc.baseline + " Td", "(" + tc.value + ") Tj"} {
				if !strings.Contains(string(sd.Content), want) {
					t.Fatalf("missing %q: %s", want, sd.Content)
				}
			}
			if _, err := Fill(ctx, out, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(out)
			b, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("later flatten changed pixels")
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
		})
	}
}

func TestAccentedSingleLineQuadding(t *testing.T) {
	for _, tc := range []struct{ q, first, encoding string }{
		{"1", "56.316 5.692 Td", "/WinAnsiEncoding"},
		{"2", "110.632 5.692 Td", "/WinAnsiEncoding"},
		{"1", "56.316 5.692 Td", "<< /Differences [201 /Eacute 224 /agrave 233 /eacute] >>"},
	} {
		t.Run(tc.q, func(t *testing.T) {
			objs := formObjs()
			objs[12] = strings.Replace(objs[12], "/FT /Tx", "/FT /Tx /Q "+tc.q, 1)
			for i := range objs {
				objs[i] = strings.ReplaceAll(objs[i], "/BaseFont /Helvetica", "/BaseFont /Helvetica /Encoding "+tc.encoding)
			}
			before := rawPDF(objs)
			source := writeTemp(t, "aligned.pdf", before)
			filled := filepath.Join(t.TempDir(), "filled.pdf")
			flat := filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			if _, err := Fill(ctx, source, filled, map[string]string{"name": "Café déjà vu", "choice": "Alpha"}, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, filled)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := textOf(entry(c.XRefTable, fields["name"].D, "V")); got != "Café déjà vu" {
				t.Fatalf("stored value changed: %q", got)
			}
			ap := dictOf(c.XRefTable, fields["name"].Widgets[0]["AP"]).d
			sd, _ := streamOf(c.XRefTable, ap["N"])
			if sd == nil {
				t.Fatal("missing appearance")
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tc.first, "(Caf\xe9 d\xe9j\xe0 vu) Tj"} {
				if !strings.Contains(string(sd.Content), want) {
					t.Fatalf("missing %q in %s", want, sd.Content)
				}
			}
			if _, err := Fill(ctx, filled, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, _ := os.ReadFile(filled)
			b, _ := os.ReadFile(flat)
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("flatten changed accented text pixels")
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

// The reference paints hidden saved appearances when flattening, even though
// the editable widget stays hidden. Compare with an otherwise visible source.
func TestFlattenHiddenSavedAppearance(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(fmt.Sprint(edited), func(t *testing.T) {
			ctx := context.Background()
			var flattened [][]byte
			for _, hidden := range []bool{false, true} {
				objs := formObjs()
				if hidden {
					objs[13] = strings.Replace(objs[13], "/F 4", "/F 6", 1)
				}
				before := rawPDF(objs)
				source := writeTemp(t, "source.pdf", before)
				out := filepath.Join(t.TempDir(), "filled.pdf")
				flat := filepath.Join(t.TempDir(), "flat.pdf")
				values := map[string]string{}
				if edited {
					values["name"] = "Ada"
				}
				if _, err := Fill(ctx, source, out, values, false); err != nil {
					t.Fatal(err)
				}
				c, _, err := LoadFile(ctx, out)
				if err != nil {
					t.Fatal(err)
				}
				fields, _, err := fieldTree(c.XRefTable)
				if err != nil {
					t.Fatal(err)
				}
				want := 4
				if hidden {
					want = 6
				}
				if got := intOf(c.XRefTable, fields["name"].Widgets[0], "F"); got != want {
					t.Fatalf("flags: %d", got)
				}
				if _, err := Fill(ctx, out, flat, nil, true); err != nil {
					t.Fatal(err)
				}
				b, err := os.ReadFile(flat)
				if err != nil {
					t.Fatal(err)
				}
				flattened = append(flattened, b)
				after, err := os.ReadFile(source)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("source changed", err)
				}
			}
			if !bytes.Equal(images(t, flattened[0])[1].Pix, images(t, flattened[1])[1].Pix) {
				t.Fatal("hidden appearance omitted during flattening")
			}
		})
	}
}

func TestRGBDefaultPrecisionAndFlatten(t *testing.T) {
	for _, q := range []string{"0", "1", "2"} {
		t.Run(q, func(t *testing.T) {
			objs := formObjs()
			for i := range objs {
				objs[i] = strings.ReplaceAll(objs[i], "/Helv 12 Tf 0 g", "/Helv 12 Tf 0.125 0.375 0.625 rg")
				objs[i] = strings.ReplaceAll(objs[i], "/FT /Tx", "/FT /Tx /Q "+q)
				objs[i] = strings.ReplaceAll(objs[i], "/FT /Ch", "/FT /Ch /Q "+q)
			}
			before := rawPDF(objs)
			source := writeTemp(t, "rgb.pdf", before)
			out, flat := filepath.Join(t.TempDir(), "filled.pdf"), filepath.Join(t.TempDir(), "flat.pdf")
			ctx := context.Background()
			values := map[string]string{"name": "Ada (RGB)", "choice": "Beta"}
			if _, err := Fill(ctx, source, out, values, false); err != nil {
				t.Fatal(err)
			}
			c, _, err := LoadFile(ctx, out)
			if err != nil {
				t.Fatal(err)
			}
			fields, _, err := fieldTree(c.XRefTable)
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range values {
				f := fields[key]
				got, _ := textOf(entry(c.XRefTable, f.D, "V"))
				if got != value {
					t.Fatalf("%s value: %q", key, got)
				}
				da, _ := textOf(entry(c.XRefTable, f.D, "DA"))
				if da != "/Helv 12 Tf 0.125 0.375 0.625 rg" {
					t.Fatalf("DA changed: %q", da)
				}
				sd, _ := streamOf(c.XRefTable, dictOf(c.XRefTable, f.Widgets[0]["AP"]).d["N"])
				if sd == nil {
					t.Fatal("missing appearance")
				}
				if err := sd.Decode(); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(sd.Content), "0.125 0.375 0.625 rg") {
					t.Fatalf("rounded color: %s", sd.Content)
				}
			}
			if _, err := Fill(ctx, out, flat, nil, true); err != nil {
				t.Fatal(err)
			}
			a, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(flat)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(images(t, a)[1].Pix, images(t, b)[1].Pix) {
				t.Fatal("flatten changed pixels")
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestRGBDefaultRejectsUnsupportedOperators(t *testing.T) {
	prefix := "BT /Helv 12 Tf 0.12 0.38 0.62 rg "
	for _, da := range []string{"/Helv 12 Tf 1.1 0 0 rg", "/Helv 12 Tf 0.5 g", "/Helv 12 Tf 0 0 0 1 k", "/Helv 12 Tf 0.1 0.2 0.3 rg 1 Tr"} {
		if got := preserveRGBDefault(prefix, da); got != prefix {
			t.Fatalf("rewrote unsupported DA %q", da)
		}
	}
}
