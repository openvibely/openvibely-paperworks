package office

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// ChartLiteral returns ordered literal points, rejecting malformed or
// missing numeric values. Missing indexes are nil.
func ChartLiteral(literal *etree.Element) ([]*string, error) {
	count := Child(literal, NSC, "ptCount")
	if count == nil {
		return nil, errors.New("literal chart needs point count")
	}
	total, err := strconv.Atoi(count.SelectAttrValue("val", "-1"))
	if err != nil || total <= 0 || total > 100000 {
		return nil, errors.New("literal chart point count outside limits")
	}
	out := make([]*string, total)
	seen := 0
	for _, pt := range Children(literal, NSC, "pt") {
		idx, err := strconv.Atoi(pt.SelectAttrValue("idx", "-1"))
		v := Child(pt, NSC, "v")
		if err != nil || idx < 0 || idx >= total || out[idx] != nil || v == nil {
			return nil, errors.New("literal chart has invalid or duplicate points")
		}
		text := v.Text()
		if literal.Tag == "numLit" {
			f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
				return nil, errors.New("literal chart number must be finite")
			}
		}
		out[idx] = &text
		seen++
	}
	if seen == 0 {
		return nil, errors.New("literal chart contains no data")
	}
	return out, nil
}

// ChartWorkbookTarget resolves the embedded workbook part behind a chart.
func ChartWorkbookTarget(p *opc.Package, name string, root *etree.Element) (string, error) {
	if Is(root, NSCX, "chartSpace") {
		return ExtendedChartWorkbook(root, name, p)
	}
	ext := Child(root, NSC, "externalData")
	if ext == nil {
		return "", errors.New(name + ": no embedded workbook")
	}
	t, err := RelTarget(p, name, AttrV(ext, NSR, "id"))
	if err != nil {
		return "", errors.New(name + ": embedded workbook is unavailable")
	}
	return t, nil
}

func chartRefs(root *etree.Element) []*etree.Element {
	return append(All(root, NSC, "numRef"), All(root, NSC, "strRef")...)
}

func openEmbedded(raw []byte) (*Workbook, error) {
	nested, err := opc.Read(raw)
	if err != nil {
		return nil, fmt.Errorf("embedded workbook: %w", err)
	}
	return OpenWorkbook(nested)
}

// VerifyChartData compares chart reference caches to saved embedded
// workbook values. targetApp "powerpoint" accepts literal-only charts.
func VerifyChartData(p *opc.Package, targetApp string) ([]any, error) {
	names, err := ChartParts(p)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, name := range names {
		doc, err := p.XML(name)
		if err != nil {
			return nil, err
		}
		root := doc.Root()
		if Is(root, NSCX, "chartSpace") {
			r, err := VerifyExtendedChart(root, name, p)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
			continue
		}
		for _, title := range All(root, NSC, "title") {
			var b strings.Builder
			for _, t := range All(title, NSA, "t") {
				b.WriteString(t.Text())
			}
			text := strings.TrimSpace(b.String())
			if text == "" {
				b.Reset()
				for _, v := range All(title, NSC, "v") {
					b.WriteString(v.Text())
				}
				text = strings.TrimSpace(b.String())
			}
			if strings.EqualFold(text, "chart title") {
				return nil, errors.New(name + ": replace placeholder Chart Title")
			}
		}
		refs := chartRefs(root)
		if len(All(root, NSC, "multiLvlStrRef")) > 0 {
			return nil, errors.New(name + ": multilevel chart references are not verified")
		}
		literals := append(All(root, NSC, "numLit"), All(root, NSC, "strLit")...)
		for _, l := range literals {
			if _, err := ChartLiteral(l); err != nil {
				return nil, err
			}
		}
		if len(literals) > 0 && targetApp != "powerpoint" {
			return nil, errors.New(name + ": literal chart data needs explicit workbook materialization or target_application=powerpoint")
		}
		if len(refs) == 0 {
			if len(literals) == 0 {
				return nil, errors.New(name + ": no verifiable chart data")
			}
			out = append(out, map[string]any{"chart": name, "references": 0, "literal_series": len(literals), "target_application": "powerpoint", "workbook_verified": false})
			continue
		}
		if Child(root, NSC, "externalData") == nil {
			return nil, errors.New(name + ": chart references have no embedded workbook")
		}
		target, err := ChartWorkbookTarget(p, name, root)
		if err != nil {
			return nil, err
		}
		if !p.Has(target) {
			return nil, errors.New(name + ": embedded workbook is unavailable")
		}
		wb, err := openEmbedded(p.Get(target))
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			if err := verifyRef(name, ref, wb); err != nil {
				return nil, err
			}
		}
		out = append(out, map[string]any{"chart": name, "workbook": target, "references": len(refs), "literal_series": len(literals)})
	}
	return out, nil
}

func refCells(formula string) (string, []string, error) {
	sheet, rng, ok := splitSheetRef(formula, chartRangeRe)
	if !ok {
		return "", nil, errors.New("unsupported chart range " + formula)
	}
	l, t, r, b, err := ParseRange(rng)
	if err != nil {
		return "", nil, err
	}
	if (r-l+1)*(b-t+1) > 100000 {
		return "", nil, errors.New("chart reference exceeds validation limit")
	}
	var cells []string
	for row := t; row <= b; row++ {
		for col := l; col <= r; col++ {
			cells = append(cells, ColumnName(col)+strconv.Itoa(row))
		}
	}
	return sheet, cells, nil
}

func verifyRef(name string, ref *etree.Element, wb *Workbook) error {
	formula := strings.TrimSpace(Child(ref, NSC, "f").Text())
	sheet, cells, err := refCells(formula)
	if err != nil {
		return errors.New(name + ": " + err.Error())
	}
	numeric := ref.Tag == "numRef"
	cacheTag := "strCache"
	if numeric {
		cacheTag = "numCache"
	}
	cache := Child(ref, NSC, cacheTag)
	if cache == nil {
		return errors.New(name + ": chart has no saved cache")
	}
	saved := map[int]string{}
	pts := Children(cache, NSC, "pt")
	for _, pt := range pts {
		i, err := strconv.Atoi(pt.SelectAttrValue("idx", ""))
		if err != nil {
			return errors.New(name + ": invalid chart cache index")
		}
		saved[i] = Text(pt, NSC, "v")
	}
	if len(saved) != len(pts) {
		return errors.New(name + ": duplicate chart cache points")
	}
	count := Child(cache, NSC, "ptCount")
	if count == nil || count.SelectAttrValue("val", "-1") != strconv.Itoa(len(cells)) {
		return errors.New(name + ": chart cache point count differs from source")
	}
	for i := range saved {
		if i < 0 || i >= len(cells) {
			return errors.New(name + ": chart cache range is inconsistent")
		}
	}
	for i, cell := range cells {
		v, err := wb.Value(sheet, cell)
		if err != nil {
			return errors.New(name + ": " + err.Error())
		}
		cached, has := saved[i]
		where := sheet + "!" + cell
		switch {
		case v.Kind == "":
			if v.Formula {
				return errors.New(name + ": embedded formula has no saved value at " + cell)
			}
			if has && cached != "" {
				return errors.New(name + ": blank source differs from chart cache")
			}
		case numeric:
			f, ok := v.Number()
			c, err := strconv.ParseFloat(strings.TrimSpace(cached), 64)
			if !ok || !has || err != nil || !isClose(f, c) {
				return errors.New(name + ": chart cache differs from " + where)
			}
		default:
			if pyStr(v) != cached {
				return errors.New(name + ": chart label differs from " + where)
			}
		}
	}
	return nil
}

func isClose(a, b float64) bool {
	return math.Abs(a-b) <= math.Max(1e-9*math.Max(math.Abs(a), math.Abs(b)), 1e-9)
}

// PyStr renders a saved cell value as Reference's str() would.
func PyStr(v CellValue) string { return pyStr(v) }
