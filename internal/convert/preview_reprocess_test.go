package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

// Saved histogram previews retain their native chart parts. Discovery still
// requests preview processing, but a second pass need not create a new file.
func TestConvertPDFSparklineSpanUsesSourceRangePositions(t *testing.T) {
	dir := t.TempDir()
	f := excelize.NewFile()
	for cell, value := range map[string]int{"A1": 10, "C1": 20, "D1": 30} {
		if err := f.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	book, err := opc.Read(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := office.EditSparklines(book, office.SparklineSpec{
		Sheet:      "Sheet1",
		Sparklines: []office.SparklineItem{{Cell: "H1", Range: "Sheet1!A1:D1"}},
		Options:    map[string]json.RawMessage{"displayEmptyCellsAs": json.RawMessage(`"span"`)},
	}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "span.xlsx")
	if err := book.SaveNew(source); err != nil {
		t.Fatal(err)
	}

	var gotX []int
	ctx := withDeps(dependencies{refresh: func(_ context.Context, input, output string, wholeSheets bool) error {
		if wholeSheets {
			return fmt.Errorf("unexpected whole-sheet PDF export")
		}
		if input == source {
			return fmt.Errorf("PDF export received the source instead of the sparkline preview")
		}
		preview, err := opc.Open(input)
		if err != nil {
			return err
		}
		drawing, err := preview.XML("xl/drawings/preview1.xml")
		if err != nil {
			return err
		}
		shapes := office.All(drawing.Root(), office.NSXDR, "sp")
		if len(shapes) != 1 {
			return fmt.Errorf("preview line shapes=%d, want 1", len(shapes))
		}
		points := office.All(shapes[0], office.NSA, "pt")
		if len(points) != 3 {
			return fmt.Errorf("preview points=%d, want 3", len(points))
		}
		anchor := shapes[0].Parent()
		ext := office.Child(anchor, office.NSXDR, "ext")
		width, err := strconv.ParseFloat(ext.SelectAttrValue("cx", ""), 64)
		if err != nil {
			return err
		}
		height, err := strconv.ParseFloat(ext.SelectAttrValue("cy", ""), 64)
		if err != nil {
			return err
		}
		pad := math.Min(28575, math.Min(width/4, height/4))
		for _, index := range []int{0, 2, 3} {
			x, err := strconv.Atoi(points[len(gotX)].SelectAttrValue("x", ""))
			if err != nil {
				return err
			}
			gotX = append(gotX, x)
			want := int(math.RoundToEven(pad + float64(index)/3*(width-2*pad)))
			if x != want {
				return fmt.Errorf("preview point %d x=%d, want source index %d at x=%d", len(gotX)-1, x, index, want)
			}
		}
		return os.WriteFile(output, []byte("PDF export sentinel"), 0600)
	}})
	if _, err := Convert(ctx, source, filepath.Join(dir, "out"), Options{}); err != nil {
		t.Fatal(err)
	}
	if len(gotX) != 3 {
		t.Fatalf("PDF export inspected %d preview points", len(gotX))
	}
}

func TestConvertSavedHistogramPreview(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "saved.pptx")
	if changed, err := office.OfficePreviewCopy("../../testdata/fixtures/reference-categorical-histogram.pptx", source); err != nil || !changed {
		t.Fatal(changed, err)
	}
	before := contents(t, source)
	if needed, err := office.NeedsOfficePreview(source); err != nil || !needed {
		t.Fatal(needed, err)
	}
	unused := filepath.Join(dir, "unused.pptx")
	if changed, err := office.OfficePreviewCopy(source, unused); err != nil || changed {
		t.Fatal(changed, err)
	}
	if _, err := os.Stat(unused); !os.IsNotExist(err) {
		t.Fatalf("unchanged preview wrote a file: %v", err)
	}
	called := false
	ctx := withDeps(dependencies{export: func(_ context.Context, input, output, target string) (string, error) {
		called = true
		if input != source {
			return "", fmt.Errorf("export input %q, want existing source %q", input, source)
		}
		if !bytes.Equal(contents(t, input), before) {
			t.Fatal("source changed before export")
		}
		if target != "pdf" {
			t.Fatal(target)
		}
		path := filepath.Join(output, "saved.pdf")
		return path, os.WriteFile(path, []byte("export-routing sentinel"), 0600)
	}})
	if _, err := Convert(ctx, source, filepath.Join(dir, "out"), Options{}); err != nil {
		t.Fatal(err)
	}
	if !called || !bytes.Equal(contents(t, source), before) {
		t.Fatal("missing export or source changed")
	}
}
