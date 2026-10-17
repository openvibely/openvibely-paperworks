package office

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/beevik/etree"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

type cacheRequest struct {
	Formula
	ResultPart string
	Clear      bool
	Child      bool
}

func rangeCells(ref string) (map[string]bool, error) {
	l, t, r, b, err := ParseRange(ref)
	if err != nil {
		return nil, err
	}
	if l < 1 || t < 1 || r > 16384 || b > 1048576 || (r-l+1)*(b-t+1) > 100000 {
		return nil, errors.New("formula result area exceeds 100000 cells")
	}
	out := map[string]bool{}
	for row := t; row <= b; row++ {
		for col := l; col <= r; col++ {
			out[ColumnName(col)+strconv.Itoa(row)] = true
		}
	}
	return out, nil
}

// PreserveFormulaCaches copies calculated results from the engine output into
// the original package, leaving formulas and every unrelated part untouched,
// then refreshes chart caches from the new results.
func PreserveFormulaCaches(source string, calculated []byte, formulas []Formula) ([]byte, error) {
	orig, err := opc.Open(source)
	if err != nil {
		return nil, err
	}
	gen, err := opc.Read(calculated)
	if err != nil {
		return nil, err
	}
	genWB, err := OpenWorkbook(gen)
	if err != nil {
		return nil, err
	}
	genRep, err := PackageReportOf(gen)
	if err != nil {
		return nil, err
	}
	byID := map[[2]string]Formula{}
	for _, f := range genRep["formulas"].([]Formula) {
		byID[[2]string{f.Sheet, f.Cell}] = f
	}
	reqs := map[[2]string]*cacheRequest{}
	var order [][2]string
	add := func(k [2]string, r *cacheRequest) {
		if _, ok := reqs[k]; !ok {
			order = append(order, k)
			reqs[k] = r
		}
	}
	for _, f := range formulas {
		res, ok := byID[[2]string{f.Sheet, f.Cell}]
		if !ok {
			return nil, fmt.Errorf("calculation lost formula %s!%s", f.Sheet, f.Cell)
		}
		add([2]string{f.Sheet, f.Cell}, &cacheRequest{Formula: f, ResultPart: res.Part})
	}
	arrayRefs := map[[2]string]string{}
	for _, f := range formulas {
		t := f.Attributes["t"]
		if t != "dataTable" && t != "array" {
			continue
		}
		res := byID[[2]string{f.Sheet, f.Cell}]
		oldRef, newRef := f.Attributes["ref"], f.Attributes["ref"]
		if t == "array" {
			if res.Attributes["t"] != "array" {
				return nil, errors.New("calculation did not retain array metadata")
			}
			newRef = res.Attributes["ref"]
			arrayRefs[[2]string{f.Part, f.Cell}] = newRef
		}
		oldA, err := rangeCells(oldRef)
		if err != nil {
			return nil, err
		}
		newA, err := rangeCells(newRef)
		if err != nil {
			return nil, err
		}
		if !newA[f.Cell] {
			return nil, errors.New("array result does not contain its anchor")
		}
		odoc, err := orig.XML(f.Part)
		if err != nil {
			return nil, err
		}
		occupied := map[string]*etree.Element{}
		for _, c := range All(odoc.Root(), NSS, "c") {
			occupied[c.SelectAttrValue("r", "")] = c
		}
		for c := range newA {
			if oldA[c] {
				continue
			}
			if o := occupied[c]; o != nil && (Child(o, NSS, "f") != nil || Child(o, NSS, "v") != nil || Child(o, NSS, "is") != nil) {
				return nil, errors.New("expanded array would overwrite an existing cell: " + c)
			}
		}
		union := map[string]bool{}
		for c := range oldA {
			union[c] = true
		}
		for c := range newA {
			union[c] = true
		}
		cells := make([]string, 0, len(union))
		for c := range union {
			cells = append(cells, c)
		}
		slices.Sort(cells)
		for _, c := range cells {
			k := [2]string{f.Sheet, c}
			if _, ok := reqs[k]; ok && c != f.Cell {
				return nil, errors.New("formula result area overlaps another formula")
			}
			child := f
			child.Cell = c
			add(k, &cacheRequest{Formula: child, ResultPart: res.Part, Clear: !newA[c], Child: true})
		}
	}
	changed := map[string]*etree.Document{}
	genSheets := map[string]*Sheet{}
	for _, s := range genWB.Sheets {
		genSheets[s.Part] = s
	}
	for _, k := range order {
		it := reqs[k]
		gs := genSheets[it.ResultPart]
		if gs == nil {
			// PackageReport also inspects partial packages without workbook
			// relationships. Use the exact part recorded for the matched formula;
			// never guess a sheet by its position or filename.
			doc, err := gen.XML(it.ResultPart)
			if err != nil || !Is(doc.Root(), NSS, "worksheet") {
				return nil, errors.New("calculated workbook is missing " + it.ResultPart)
			}
			gs = &Sheet{Part: it.ResultPart, Doc: doc, Cells: map[string]*etree.Element{}}
			for _, c := range All(doc.Root(), NSS, "c") {
				gs.Cells[strings.ToUpper(c.SelectAttrValue("r", ""))] = c
			}
			genSheets[it.ResultPart] = gs
		}
		cell := gs.Cells[it.Cell]
		clear := it.Clear
		var val CellValue
		if cell != nil {
			val, err = genWB.CellValueOf(cell)
			if err != nil {
				return nil, err
			}
		}
		hasValue := cell != nil && (Child(cell, NSS, "v") != nil || Child(cell, NSS, "is") != nil)
		if !hasValue && !clear && !it.Child {
			return nil, fmt.Errorf("calculation produced no saved result for %s!%s", it.Sheet, it.Cell)
		}
		if !hasValue {
			clear = true
		}
		if !clear && !slices.Contains([]string{"n", "str", "b", "e"}, val.Kind) {
			return nil, errors.New("unsupported calculated value type: " + val.Kind)
		}
		doc := changed[it.Part]
		if doc == nil {
			if doc, err = orig.XML(it.Part); err != nil {
				return nil, err
			}
			changed[it.Part] = doc
		}
		target, err := ensureCell(doc.Root(), it.Cell)
		if err != nil {
			return nil, err
		}
		for _, c := range target.ChildElements() {
			if (Is(c, NSS, "v") || Is(c, NSS, "is")) && c.Parent() == target {
				target.RemoveChild(c)
			}
		}
		if clear {
			target.RemoveAttr("t")
			continue
		}
		if val.Kind == "n" {
			target.RemoveAttr("t")
		} else {
			target.CreateAttr("t", val.Kind)
		}
		v := etree.NewElement(qual(target.Space, "v"))
		v.SetText(val.Text)
		f := Child(target, NSS, "f")
		if nr, ok := arrayRefs[[2]string{it.Part, it.Cell}]; ok && f != nil {
			f.CreateAttr("ref", nr)
		}
		if f != nil {
			target.InsertChildAt(f.Index()+1, v)
		} else {
			target.InsertChildAt(0, v)
		}
	}
	for part, doc := range changed {
		if dim := Child(doc.Root(), NSS, "dimension"); dim != nil {
			l, t, r, b, err := ParseRange(dim.SelectAttrValue("ref", "A1"))
			if err == nil {
				for _, c := range All(doc.Root(), NSS, "c") {
					col, row, err := SplitCell(c.SelectAttrValue("r", ""))
					if err != nil {
						continue
					}
					l, t, r, b = min(l, col), min(t, row), max(r, col), max(b, row)
				}
				dim.CreateAttr("ref", fmt.Sprintf("%s%d:%s%d", ColumnName(l), t, ColumnName(r), b))
			}
		}
		data, err := formulaCacheXML(doc)
		if err != nil {
			return nil, err
		}
		orig.Set(part, data)
	}
	if err := RefreshWorkbookChartCaches(orig); err != nil {
		return nil, err
	}
	return orig.Bytes()
}

// ensureCell finds or creates the c element for ref in row/column order.
func ensureCell(ws *etree.Element, ref string) (*etree.Element, error) {
	data := Child(ws, NSS, "sheetData")
	if data == nil {
		return nil, errors.New("worksheet has no sheetData")
	}
	col, rowN, err := SplitCell(ref)
	if err != nil {
		return nil, err
	}
	var row *etree.Element
	for _, r := range Children(data, NSS, "row") {
		// Row indexes are optional in partial packages accepted by inspection.
		// Preserve an existing addressed cell instead of creating a duplicate.
		if r.SelectAttrValue("r", "") == "" {
			for _, c := range Children(r, NSS, "c") {
				if strings.EqualFold(c.SelectAttrValue("r", ""), ref) {
					return c, nil
				}
			}
		}
		n, _ := strconv.Atoi(r.SelectAttrValue("r", "0"))
		if n == rowN {
			row = r
			break
		}
		if n > rowN {
			row = etree.NewElement(qual(data.Space, "row"))
			row.CreateAttr("r", strconv.Itoa(rowN))
			data.InsertChildAt(r.Index(), row)
			break
		}
	}
	if row == nil {
		row = data.CreateElement(qual(data.Space, "row"))
		row.CreateAttr("r", strconv.Itoa(rowN))
	}
	for _, c := range Children(row, NSS, "c") {
		r := c.SelectAttrValue("r", "")
		if strings.EqualFold(r, ref) {
			return c, nil
		}
		cc, _, err := SplitCell(r)
		if err == nil && cc > col {
			nc := etree.NewElement(qual(row.Space, "c"))
			nc.CreateAttr("r", ref)
			row.InsertChildAt(c.Index(), nc)
			return nc, nil
		}
	}
	nc := row.CreateElement(qual(row.Space, "c"))
	nc.CreateAttr("r", ref)
	return nc, nil
}

var chartMimes = []string{"application/vnd.openxmlformats-officedocument.drawingml.chart+xml", "application/vnd.ms-office.chartex+xml"}

const NSCX = "http://schemas.microsoft.com/office/drawing/2014/chartex"

// ChartParts discovers chart parts by content type and relationships.
func ChartParts(p *opc.Package) ([]string, error) {
	found := map[string]bool{}
	for _, n := range p.Names() {
		if slices.Contains(chartMimes, p.ContentType(n)) {
			found[n] = true
		}
		if !strings.HasSuffix(n, ".rels") {
			continue
		}
		owner := opc.SourceOf(n)
		rels, err := p.Rels(owner)
		if err != nil {
			return nil, err
		}
		for _, r := range rels {
			last := r.Type[strings.LastIndex(r.Type, "/")+1:]
			if last != "chart" && last != "chartEx" {
				continue
			}
			if r.TargetMode == "External" {
				return nil, errors.New("external chart part is unsupported")
			}
			found[opc.Resolve(owner, r.Target)] = true
		}
	}
	var out []string
	for n := range found {
		if !p.Has(n) {
			return nil, errors.New("Missing chart part: " + n)
		}
		doc, err := p.XML(n)
		if err != nil {
			return nil, err
		}
		if !Is(doc.Root(), NSC, "chartSpace") && !Is(doc.Root(), NSCX, "chartSpace") {
			return nil, errors.New("invalid chart part: " + n)
		}
		out = append(out, n)
	}
	slices.SortFunc(out, naturalCompare)
	return out, nil
}

var digitsRe = regexp.MustCompile(`\d+|\D+`)

func naturalCompare(a, b string) int {
	pa, pb := digitsRe.FindAllString(a, -1), digitsRe.FindAllString(b, -1)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		if ea == nil && eb == nil {
			if na != nb {
				return na - nb
			}
			continue
		}
		if c := strings.Compare(pa[i], pb[i]); c != 0 {
			return c
		}
	}
	return len(pa) - len(pb)
}

var chartRangeRe = regexp.MustCompile(`^(?:'((?:[^']|'')+)'|([^!]+))!(\$?[A-Z]+\$?\d+(?::\$?[A-Z]+\$?\d+)?)$`)

// RefreshWorkbookChartCaches rewrites standard chart caches in a workbook
// from its saved cell values.
func RefreshWorkbookChartCaches(p *opc.Package) error {
	charts, err := ChartParts(p)
	if err != nil || len(charts) == 0 {
		return err
	}
	wb, err := OpenWorkbook(p)
	if err != nil {
		return err
	}
	for _, name := range charts {
		doc, err := p.XML(name)
		if err != nil {
			return err
		}
		if !Is(doc.Root(), NSC, "chartSpace") {
			return errors.New("extended workbook chart cache refresh is unsupported")
		}
		if err := refreshChartReferences(doc.Root(), wb, true); err != nil {
			return err
		}
		raw, err := workbookChartCacheXML(doc)
		if err != nil {
			return err
		}
		p.Set(name, raw)
	}
	return nil
}

// workbookChartCacheXML follows the reference chart-cache writer's root-only
// UTF-8 declaration and text quoting. It does not change general OPC writing.
func workbookChartCacheXML(source *etree.Document) ([]byte, error) {
	doc := etree.NewDocument()
	doc.SetRoot(source.Root().Copy())
	doc.WriteSettings.CanonicalText = true
	raw, err := doc.WriteToBytes()
	if err != nil {
		return nil, err
	}
	return append([]byte("<?xml version='1.0' encoding='UTF-8'?>\n"), raw...), nil
}

// RefreshChartReferences rebuilds every numRef/strRef cache under root.
func RefreshChartReferences(root *etree.Element, wb *Workbook) error {
	return refreshChartReferences(root, wb, false)
}

func refreshChartReferences(root *etree.Element, wb *Workbook, workbookEmptyStrings bool) error {
	if len(All(root, NSC, "multiLvlStrRef")) > 0 {
		return errors.New("multilevel chart caches require explicit refresh support")
	}
	refs := append(All(root, NSC, "numRef"), All(root, NSC, "strRef")...)
	for _, ref := range refs {
		formula := strings.TrimSpace(Text(Child(ref, NSC, "f"), NSC, "f"))
		m := chartRangeRe.FindStringSubmatch(formula)
		if m == nil {
			return errors.New("unsupported chart range " + formula)
		}
		sheet := m[2]
		if m[1] != "" {
			sheet = strings.ReplaceAll(m[1], "''", "'")
		}
		l, t, r, b, err := ParseRange(m[3])
		if err != nil {
			return err
		}
		if (r-l+1)*(b-t+1) > 100000 {
			return errors.New("chart range exceeds limits")
		}
		var vals []CellValue
		for row := t; row <= b; row++ {
			for col := l; col <= r; col++ {
				v, err := wb.Value(sheet, ColumnName(col)+strconv.Itoa(row))
				if err != nil {
					return err
				}
				// The reference workbook refresh patches explicit empty string-formula
				// caches after loading. Trace and other readers still expose null.
				if workbookEmptyStrings && v.Formula && v.Kind == "" {
					cell := wb.Sheets[sheet].Cells[ColumnName(col)+strconv.Itoa(row)]
					cached := Child(cell, NSS, "v")
					if cell.SelectAttrValue("t", "") == "str" && cached != nil && cached.Text() == "" {
						v.Kind = "str"
					}
				}
				vals = append(vals, v)
			}
		}
		numeric := ref.Tag == "numRef"
		if numeric && Is(ref.Parent(), NSC, "cat") && slices.ContainsFunc(vals, func(v CellValue) bool { return v.Kind == "str" }) {
			ref.Tag = "strRef"
			numeric = false
			RemoveAll(ref, NSC, "numCache")
		}
		cacheTag := "strCache"
		if numeric {
			cacheTag = "numCache"
		}
		cache := Child(ref, NSC, cacheTag)
		if cache == nil {
			cache = NewChild(ref, NSC, cacheTag, []string{"extLst"})
		}
		RemoveAll(cache, NSC, "pt")
		RemoveAll(cache, NSC, "ptCount")
		later := []string{"extLst"}
		pc := NewChild(cache, NSC, "ptCount", later)
		pc.CreateAttr("val", strconv.Itoa(len(vals)))
		for i, v := range vals {
			if v.Kind == "e" {
				return errors.New("chart source contains calculation errors")
			}
			if v.Kind == "" {
				if v.Formula {
					return errors.New("chart formula has no saved result")
				}
				continue
			}
			text := v.Text
			if numeric {
				if v.Kind == "str" && v.Text == "" {
					continue
				}
				f, ok := v.Number()
				if !ok || math.IsInf(f, 0) || math.IsNaN(f) {
					return errors.New("chart source is not finite numeric data")
				}
				text = strconv.FormatFloat(f, 'f', -1, 64)
			}
			pt := NewChild(cache, NSC, "pt", later)
			pt.CreateAttr("idx", strconv.Itoa(i))
			NewChild(pt, NSC, "v", nil).SetText(text)
		}
	}
	return nil
}

func cellAt(l, t, r, i int) string {
	w := r - l + 1
	return ColumnName(l+i%w) + strconv.Itoa(t+i/w)
}

// The reference cache writer uses minidom.Document.toxml(encoding="UTF-8"):
// a fresh declaration without standalone/newline, even if the source had them.
// Notes repair also uses this minidom policy; other OOXML writers differ.
func formulaCacheXML(source *etree.Document) ([]byte, error) {
	doc := source.Copy()
	for i := len(doc.Child) - 1; i >= 0; i-- {
		switch n := doc.Child[i].(type) {
		case *etree.ProcInst:
			if n.Target == "xml" {
				doc.RemoveChildAt(i)
			}
		case *etree.CharData:
			// minidom does not retain XML document-level whitespace/BOM as text nodes.
			doc.RemoveChildAt(i)
		}
	}
	doc.WriteSettings.CanonicalEndTags = false
	raw, err := doc.WriteToBytes()
	if err != nil {
		return nil, err
	}
	return append([]byte(`<?xml version="1.0" encoding="UTF-8"?>`), raw...), nil
}
