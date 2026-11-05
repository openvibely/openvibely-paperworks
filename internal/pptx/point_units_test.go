package pptx

import (
	"math"
	"testing"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func pointTestElement(t *testing.T, xml string) *etree.Element {
	t.Helper()
	doc := etree.NewDocument()
	if err := doc.ReadFromString(xml); err != nil {
		t.Fatal(err)
	}
	return doc.Root()
}

func pointTestTable(t *testing.T) *etree.Element {
	t.Helper()
	return pointTestElement(t, `<p:graphicFrame xmlns:p="`+pns+`" xmlns:a="`+ans+`">
		<p:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></p:xfrm>
		<a:tbl>
			<a:tblGrid><a:gridCol w="12700"/><a:gridCol w="12700"/></a:tblGrid>
			<a:tr h="12700"><a:tc><a:txBody/><a:tcPr/></a:tc><a:tc><a:txBody/><a:tcPr/></a:tc></a:tr>
			<a:tr h="12700"><a:tc><a:txBody/><a:tcPr/></a:tc><a:tc><a:txBody/><a:tcPr/></a:tc></a:tr>
		</a:tbl>
	</p:graphicFrame>`)
}

func TestPointConversionsPreserveFieldRounding(t *testing.T) {
	for _, tc := range []struct {
		name      string
		value     float64
		truncated string
		rounded   string
	}{
		{name: "integer", value: 2, truncated: "25400", rounded: "25400"},
		{name: "fractional", value: 1.00004, truncated: "12700", rounded: "12701"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shape := pointTestElement(t, `<p:sp xmlns:p="`+pns+`" xmlns:a="`+ans+`"><p:txBody><a:bodyPr/></p:txBody></p:sp>`)
			value := tc.value
			if err := styleTextBox(shape, &TextBoxStyle{Insets: &TextInsets{Top: &value, Left: &value}}); err != nil {
				t.Fatal(err)
			}
			bodyPr := office.Child(office.Child(shape, pns, "txBody"), ans, "bodyPr")
			for _, attr := range []string{"tIns", "lIns"} {
				if got := bodyPr.SelectAttrValue(attr, ""); got != tc.truncated {
					t.Errorf("text inset %s = %s; want %s", attr, got, tc.truncated)
				}
			}

			paragraph := pointTestElement(t, `<a:p xmlns:a="`+ans+`"/>`)
			if err := styleParagraph(paragraph, &ParagraphStyle{Indent: &value, MarginLeft: &value}); err != nil {
				t.Fatal(err)
			}
			pPr := office.Child(paragraph, ans, "pPr")
			for _, attr := range []string{"indent", "marL"} {
				if got := pPr.SelectAttrValue(attr, ""); got != tc.truncated {
					t.Errorf("paragraph %s = %s; want %s", attr, got, tc.truncated)
				}
			}

			textProperties := pointTestElement(t, `<a:rPr xmlns:a="`+ans+`"/>`)
			if err := paintTextProperties(textProperties, &TextPaint{Outline: &TextOutline{Width: value, Fill: &DrawingStyle{Fill: "000000"}}}); err != nil {
				t.Fatal(err)
			}
			if got := office.Child(textProperties, ans, "ln").SelectAttrValue("w", ""); got != tc.truncated {
				t.Errorf("text outline width = %s; want %s", got, tc.truncated)
			}

			shapeProperties := pointTestElement(t, `<p:sp xmlns:p="`+pns+`" xmlns:a="`+ans+`"><p:spPr/></p:sp>`)
			if err := styleDrawing(shapeProperties, &DrawingStyle{Outline: &Outline{Color: "000000", Width: value}}); err != nil {
				t.Fatal(err)
			}
			line := office.Child(office.Child(shapeProperties, pns, "spPr"), ans, "ln")
			if got := line.SelectAttrValue("w", ""); got != tc.truncated {
				t.Errorf("shape outline width = %s; want %s", got, tc.truncated)
			}

			shadowShape := pointTestElement(t, `<p:sp xmlns:p="`+pns+`" xmlns:a="`+ans+`"><p:spPr/></p:sp>`)
			if err := styleDrawing(shadowShape, &DrawingStyle{Shadow: &Shadow{Color: "000000", Blur: value, Distance: value}}); err != nil {
				t.Fatal(err)
			}
			shadow := office.All(shadowShape, ans, "outerShdw")[0]
			for _, attr := range []string{"blurRad", "dist"} {
				if got := shadow.SelectAttrValue(attr, ""); got != tc.truncated {
					t.Errorf("shadow %s = %s; want %s", attr, got, tc.truncated)
				}
			}

			table := pointTestTable(t)
			if err := formatTable(nil, "", table, &TableFormat{
				RowHeights:   []float64{2, value},
				ColumnWidths: []float64{2, value},
				Cells: []TableCellFormat{{Row: 0, Column: 0, Margins: &TextInsets{
					Top: &value, Left: &value,
				}}},
			}); err != nil {
				t.Fatal(err)
			}
			rows := office.Children(office.All(table, ans, "tbl")[0], ans, "tr")
			cols := office.Children(office.Child(office.All(table, ans, "tbl")[0], ans, "tblGrid"), ans, "gridCol")
			wantDimensions := tc.rounded
			if got := rows[0].SelectAttrValue("h", ""); got != "25400" {
				t.Errorf("integer row height = %s; want 25400", got)
			}
			if got := rows[1].SelectAttrValue("h", ""); got != wantDimensions {
				t.Errorf("fractional row height = %s; want %s", got, wantDimensions)
			}
			if got := cols[0].SelectAttrValue("w", ""); got != "25400" {
				t.Errorf("integer column width = %s; want 25400", got)
			}
			if got := cols[1].SelectAttrValue("w", ""); got != wantDimensions {
				t.Errorf("fractional column width = %s; want %s", got, wantDimensions)
			}
			cellPr := office.Child(office.Children(rows[0], ans, "tc")[0], ans, "tcPr")
			for _, attr := range []string{"marT", "marL"} {
				if got := cellPr.SelectAttrValue(attr, ""); got != tc.truncated {
					t.Errorf("cell margin %s = %s; want %s", attr, got, tc.truncated)
				}
			}
		})
	}
}

func TestPointConversionsRetainValidation(t *testing.T) {
	invalid := math.NaN()

	textBox := pointTestElement(t, `<p:sp xmlns:p="`+pns+`" xmlns:a="`+ans+`"><p:txBody><a:bodyPr/></p:txBody></p:sp>`)
	if err := styleTextBox(textBox, &TextBoxStyle{Insets: &TextInsets{Top: &invalid}}); err == nil || err.Error() != "text insets must be 0..1000 points" {
		t.Fatalf("invalid text inset error = %v", err)
	}

	paragraph := pointTestElement(t, `<a:p xmlns:a="`+ans+`"/>`)
	tooLarge := 1001.0
	if err := styleParagraph(paragraph, &ParagraphStyle{Indent: &tooLarge}); err == nil || err.Error() != "invalid indent/margin" {
		t.Fatalf("invalid paragraph indent error = %v", err)
	}

	textProperties := pointTestElement(t, `<a:rPr xmlns:a="`+ans+`"/>`)
	tooWide := 101.0
	if err := paintTextProperties(textProperties, &TextPaint{Outline: &TextOutline{Width: tooWide}}); err == nil || err.Error() != "text outline width must be 0..100 points" {
		t.Fatalf("invalid text outline error = %v", err)
	}

	shape := pointTestElement(t, `<p:sp xmlns:p="`+pns+`" xmlns:a="`+ans+`"><p:spPr/></p:sp>`)
	negative := -1.0
	if err := styleDrawing(shape, &DrawingStyle{Outline: &Outline{Color: "000000", Width: negative}}); err == nil || err.Error() != "outline width must be 0..100 points" {
		t.Fatalf("invalid shape outline error = %v", err)
	}

	shadowShape := pointTestElement(t, `<p:sp xmlns:p="`+pns+`" xmlns:a="`+ans+`"><p:spPr/></p:sp>`)
	if err := styleDrawing(shadowShape, &DrawingStyle{Shadow: &Shadow{Color: "000000", Blur: invalid}}); err == nil || err.Error() != "invalid shadow" {
		t.Fatalf("invalid shadow error = %v", err)
	}

	margins := &TextInsets{Right: &tooLarge}
	if err := formatTable(nil, "", pointTestTable(t), &TableFormat{Cells: []TableCellFormat{{Row: 0, Column: 0, Margins: margins}}}); err == nil || err.Error() != "invalid cell margin" {
		t.Fatalf("invalid cell margin error = %v", err)
	}
	for _, axis := range []string{"row", "column"} {
		t.Run(axis, func(t *testing.T) {
			style := &TableFormat{}
			if axis == "row" {
				style.RowHeights = []float64{invalid, 1}
			} else {
				style.ColumnWidths = []float64{invalid, 1}
			}
			if err := formatTable(nil, "", pointTestTable(t), style); err == nil || err.Error() != "invalid table size in points" {
				t.Fatalf("invalid table %s error = %v", axis, err)
			}
		})
	}
}

func TestPointToEMUExplicitRounding(t *testing.T) {
	for _, tc := range []struct {
		points float64
		mode   pointRounding
		want   int64
	}{
		{points: 2, mode: truncatePointEMU, want: 25400},
		{points: 1.00004, mode: truncatePointEMU, want: 12700},
		{points: 1.00004, mode: roundPointEMU, want: 12701},
		{points: -1.00004, mode: roundPointEMU, want: -12701},
	} {
		if got := pointsToEMU(tc.points, tc.mode); got != tc.want {
			t.Errorf("pointsToEMU(%g, %d) = %d; want %d", tc.points, tc.mode, got, tc.want)
		}
	}
}
