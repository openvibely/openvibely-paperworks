package office

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// ChartExDims holds one ChartEx data set: string and numeric dimensions by type
// (cat, size, val, ...) plus multilevel category paths (leaf first).
type ChartExDims struct {
	Str   map[string][]string    `json:"-"`
	Num   map[string][]float64   `json:"-"`
	Paths [][]string             `json:"category_paths,omitempty"`
	All   map[string]interface{} `json:"dimensions"`
}

func (d *ChartExDims) keys() []string {
	var k []string
	for n := range d.Str {
		k = append(k, n)
	}
	for n := range d.Num {
		k = append(k, n)
	}
	slices.Sort(k)
	return k
}

type ChartExSeries struct {
	Type       string         `json:"type"`
	DataID     string         `json:"data_id"`
	Dimensions map[string]any `json:"dimensions"`
	dims       *ChartExDims
	node       *etree.Element
}

type ChartExReport struct {
	Chart            string          `json:"chart"`
	Extended         bool            `json:"extended"`
	LiteralSeries    int             `json:"literal_series"`
	Series           []ChartExSeries `json:"series"`
	WorkbookVerified bool            `json:"workbook_verified"`
	Workbook         string          `json:"workbook,omitempty"`
	References       int             `json:"references,omitempty"`
}

var chartExRangeRe = regexp.MustCompile(`^(?:'((?:[^']|'')+)'|([^!]+))!(\$?[A-Za-z]+\$?[1-9][0-9]*(?::\$?[A-Za-z]+\$?[1-9][0-9]*)?)$`)

func splitSheetRef(text string, re *regexp.Regexp) (string, string, bool) {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	sheet := m[2]
	if m[1] != "" {
		sheet = strings.ReplaceAll(m[1], "''", "'")
	}
	return sheet, m[3], true
}

// pyStr renders a saved cell value the way Reference's str() shows it.
func pyStr(v CellValue) string {
	switch v.Kind {
	case "":
		return ""
	case "b":
		if v.Text == "1" {
			return "True"
		}
		return "False"
	case "n":
		f, err := strconv.ParseFloat(v.Text, 64)
		if err != nil {
			return v.Text
		}
		if f == math.Trunc(f) && !strings.ContainsAny(v.Text, ".eE") && math.Abs(f) < 1e16 {
			return strconv.FormatInt(int64(f), 10)
		}
		return pyRepr(f)
	}
	return v.Text
}

func pyRepr(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	exp := 0
	if f != 0 {
		exp = int(math.Floor(math.Log10(math.Abs(f))))
	}
	if exp >= -4 && exp < 16 {
		s = strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
	}
	return s
}

// writeChartExPoints writes ordered, preformatted texts into a new level.
// newPoint appends a point using the caller's namespace convention.
func writeChartExPoints(lvl *etree.Element, texts []string, newPoint func() *etree.Element) {
	lvl.CreateAttr("ptCount", strconv.Itoa(len(texts)))
	for i, text := range texts {
		pt := newPoint()
		pt.CreateAttr("idx", strconv.Itoa(i))
		pt.SetText(text)
	}
}

// ExtendedChartCache compares (or with refresh rebuilds) ChartEx dimension
// caches against a calculated workbook; it returns the number of references.
func ExtendedChartCache(root *etree.Element, wb *Workbook, refresh bool) (int, error) {
	count := 0
	data := Child(root, NSCX, "chartData")
	for _, set := range Children(data, NSCX, "data") {
		for _, dim := range set.ChildElements() {
			f := Child(dim, NSCX, "f")
			if f == nil {
				continue
			}
			text := f.Text()
			sheet, ref, ok := splitSheetRef(text, chartExRangeRe)
			if !ok {
				return 0, errors.New("unsupported ChartEx range " + text)
			}
			if wb.Sheets[sheet] == nil {
				return 0, errors.New("missing ChartEx worksheet")
			}
			l, t, r, b, err := ParseRange(strings.ToUpper(ref))
			if err != nil || r > 16384 || b > 1048576 || (r-l+1)*(b-t+1) > 100000 {
				return 0, errors.New("ChartEx range exceeds limits")
			}
			dir := f.SelectAttrValue("dir", "col")
			if dir != "col" && dir != "row" {
				return 0, errors.New("invalid ChartEx range orientation")
			}
			var groups [][]string
			if dir == "col" {
				for c := l; c <= r; c++ {
					var g []string
					for row := t; row <= b; row++ {
						g = append(g, ColumnName(c)+strconv.Itoa(row))
					}
					groups = append(groups, g)
				}
			} else {
				for row := t; row <= b; row++ {
					var g []string
					for c := l; c <= r; c++ {
						g = append(g, ColumnName(c)+strconv.Itoa(row))
					}
					groups = append(groups, g)
				}
			}
			numeric := Is(dim, NSCX, "numDim")
			if numeric && len(groups) != 1 {
				return 0, errors.New("numeric ChartEx dimension must be one row or column")
			}
			caches := Children(dim, NSCX, "lvl")
			if !refresh && len(caches) != len(groups) {
				return 0, errors.New("ChartEx hierarchy differs from workbook")
			}
			if refresh {
				for _, c := range caches {
					dim.RemoveChild(c)
				}
			}
			for gi, cells := range groups {
				var nums []float64
				var strs []string
				for _, cell := range cells {
					v, err := wb.Value(sheet, cell)
					if err != nil {
						return 0, err
					}
					if v.Kind == "e" || (v.Kind == "" && v.Formula) {
						return 0, errors.New("ChartEx source has missing or erroneous calculated value")
					}
					if numeric {
						n, ok := v.Number()
						if !ok || math.IsInf(n, 0) || math.IsNaN(n) {
							return 0, errors.New("ChartEx numeric source must be finite")
						}
						nums = append(nums, n)
					} else {
						strs = append(strs, pyStr(v))
					}
				}
				n := max(len(nums), len(strs))
				if refresh {
					lvl := etree.NewElement("lvl")
					lvl.Space = dim.Space
					if gi < len(caches) {
						for _, a := range caches[gi].Attr {
							lvl.CreateAttr(a.FullKey(), a.Value)
						}
					}
					texts := strs
					if numeric {
						texts = make([]string, len(nums))
						for i, v := range nums {
							texts[i] = pyRepr(v)
						}
					}
					writeChartExPoints(lvl, texts, func() *etree.Element {
						pt := lvl.CreateElement("pt")
						pt.Space = dim.Space
						return pt
					})
					if ext := Child(dim, NSCX, "extLst"); ext != nil {
						dim.InsertChildAt(ext.Index(), lvl)
					} else {
						dim.AddChild(lvl)
					}
					continue
				}
				lvl := caches[gi]
				pts := Children(lvl, NSCX, "pt")
				cached := map[int]string{}
				for _, p := range pts {
					i, _ := strconv.Atoi(p.SelectAttrValue("idx", "-1"))
					cached[i] = p.Text()
				}
				pc, _ := strconv.Atoi(lvl.SelectAttrValue("ptCount", "-1"))
				full := len(cached) == n
				for i := 0; i < n && full; i++ {
					_, full = cached[i]
				}
				if pc != n || len(pts) != len(cached) || !full {
					return 0, errors.New("ChartEx cache extent differs from workbook")
				}
				for i := 0; i < n; i++ {
					equal := false
					if numeric {
						c, err := strconv.ParseFloat(strings.TrimSpace(cached[i]), 64)
						equal = err == nil && math.Abs(c-nums[i]) <= math.Max(1e-9*math.Max(math.Abs(c), math.Abs(nums[i])), 1e-9)
					} else {
						equal = cached[i] == strs[i]
					}
					if !equal {
						return 0, errors.New("ChartEx cache differs from workbook " + text)
					}
				}
			}
			count++
		}
	}
	return count, nil
}

// ExtendedChartWorkbook resolves the embedded workbook part of a ChartEx chart.
func ExtendedChartWorkbook(root *etree.Element, name string, p *opc.Package) (string, error) {
	ext := Child(Child(root, NSCX, "chartData"), NSCX, "externalData")
	if ext == nil {
		return "", errors.New(name + ": ChartEx formulas need an embedded workbook")
	}
	rels, err := p.Rels(name)
	if err != nil {
		return "", err
	}
	id := AttrV(ext, NSR, "id")
	for _, r := range rels {
		if r.ID == id && r.TargetMode != "External" {
			t := opc.Resolve(name, r.Target)
			if !strings.HasSuffix(t, ".xlsx") {
				return "", errors.New("ChartEx requires an XLSX workbook")
			}
			return t, nil
		}
	}
	return "", errors.New(name + ": embedded workbook is unavailable")
}

func parseFloatStrict(s string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

// VerifyExtendedChart validates the ChartEx types supported by the workflow.
func VerifyExtendedChart(root *etree.Element, name string, p *opc.Package) (*ChartExReport, error) {
	fail := func(msg string) (*ChartExReport, error) { return nil, errors.New(name + ": " + msg) }
	datasets := map[string]*ChartExDims{}
	formulaCount := len(All(root, NSCX, "f"))
	for _, data := range Children(Child(root, NSCX, "chartData"), NSCX, "data") {
		id, ok := HasAttr(data, "id")
		if !ok || datasets[id] != nil {
			return fail("missing/duplicate ChartEx data ID")
		}
		dims := &ChartExDims{Str: map[string][]string{}, Num: map[string][]float64{}}
		for _, dim := range data.ChildElements() {
			if !Is(dim, NSCX, "strDim") && !Is(dim, NSCX, "numDim") {
				return fail("unsupported ChartEx dimension")
			}
			if len(All(dim, NSCX, "f")) > 0 && p == nil {
				return fail("ChartEx workbook formulas require an embedded package")
			}
			kind := dim.SelectAttrValue("type", "")
			levels := Children(dim, NSCX, "lvl")
			_, dupS := dims.Str[kind]
			_, dupN := dims.Num[kind]
			if len(levels) == 0 || dupS || dupN || (len(levels) > 1 && (!Is(dim, NSCX, "strDim") || kind != "cat")) {
				return fail("invalid multilevel/duplicate ChartEx dimension")
			}
			var strLevels [][]string
			var nums []float64
			for _, lvl := range levels {
				count, _ := strconv.Atoi(lvl.SelectAttrValue("ptCount", "-1"))
				if count <= 0 || count > 100000 {
					return fail("invalid ChartEx point count")
				}
				svals := make([]string, count)
				nvals := make([]float64, count)
				seen := map[int]bool{}
				for _, pt := range Children(lvl, NSCX, "pt") {
					idx, err := strconv.Atoi(pt.SelectAttrValue("idx", "-1"))
					if err != nil || seen[idx] || idx < 0 || idx >= count {
						return fail("invalid ChartEx point")
					}
					seen[idx] = true
					if Is(dim, NSCX, "numDim") {
						v, err := parseFloatStrict(pt.Text())
						if err != nil {
							return fail("invalid ChartEx number")
						}
						if math.IsInf(v, 0) || math.IsNaN(v) {
							return fail("nonfinite ChartEx number")
						}
						nvals[idx] = v
					} else {
						svals[idx] = pt.Text()
					}
				}
				if len(seen) != count {
					return fail("missing ChartEx points")
				}
				strLevels = append(strLevels, svals)
				nums = nvals
			}
			for _, l := range strLevels {
				if len(l) != len(strLevels[0]) {
					return fail("unequal hierarchy levels")
				}
			}
			if Is(dim, NSCX, "numDim") {
				dims.Num[kind] = nums
			} else {
				dims.Str[kind] = strLevels[len(strLevels)-1]
				if len(strLevels) > 1 {
					for i := range strLevels[0] {
						var path []string
						for _, l := range strLevels {
							path = append(path, l[i])
						}
						dims.Paths = append(dims.Paths, path)
					}
				}
			}
		}
		datasets[id] = dims
	}
	report := &ChartExReport{Chart: name, Extended: true}
	if Child(Child(root, NSCX, "chartData"), NSCX, "externalData") != nil || formulaCount > 0 {
		if p == nil {
			return fail("ChartEx workbook verification requires a package")
		}
		part, err := ExtendedChartWorkbook(root, name, p)
		if err != nil {
			return nil, err
		}
		raw := p.Get(part)
		if raw == nil {
			return fail("embedded workbook is missing")
		}
		nested, err := opc.Read(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: embedded workbook: %w", name, err)
		}
		wb, err := OpenWorkbook(nested)
		if err != nil {
			return nil, err
		}
		refs, err := ExtendedChartCache(root, wb, false)
		if err != nil {
			return nil, err
		}
		if refs == 0 {
			return nil, errors.New("ChartEx workbook is attached but no dimension references it")
		}
		if refs != formulaCount {
			return nil, errors.New("ChartEx contains unsupported or duplicate non-dimension formulas")
		}
		report.WorkbookVerified, report.Workbook, report.References = true, part, refs
	}
	series := Children(Child(Child(Child(root, NSCX, "chart"), NSCX, "plotArea"), NSCX, "plotAreaRegion"), NSCX, "series")
	if len(series) == 0 {
		return fail("ChartEx has no series")
	}
	for _, s := range series {
		did := Child(s, NSCX, "dataId")
		var dims *ChartExDims
		if did != nil {
			dims = datasets[did.SelectAttrValue("val", "")]
		}
		if dims == nil {
			return fail("unknown ChartEx data ID")
		}
		keys := strings.Join(dims.keys(), ",")
		layout := s.SelectAttrValue("layoutId", "")
		binning := Child(Child(s, NSCX, "layoutPr"), NSCX, "binning")
		var kind string
		switch {
		case layout == "treemap" || layout == "sunburst":
			size := dims.Num["size"]
			if keys != "cat,size" || len(dims.Str["cat"]) != len(size) || slices.ContainsFunc(size, func(v float64) bool { return v < 0 }) || !slices.ContainsFunc(size, func(v float64) bool { return v != 0 }) {
				return fail("invalid treemap categories/sizes")
			}
			kind = layout
		case layout == "waterfall" || layout == "funnel":
			val, ok := dims.Num["val"]
			if (keys != "val" && keys != "cat,val") || !ok || len(val) == 0 {
				return fail(layout + " requires values with optional categories")
			}
			if cat, has := dims.Str["cat"]; has && len(cat) != len(val) {
				return fail(layout + " categories must match values")
			}
			if layout == "funnel" && (slices.ContainsFunc(val, func(v float64) bool { return v < 0 }) || !slices.ContainsFunc(val, func(v float64) bool { return v != 0 })) {
				return fail("funnel requires nonnegative values with a positive maximum")
			}
			kind = layout
		case layout == "clusteredColumn" && binning != nil:
			_, ok := dims.Num["val"]
			if (keys != "val" && keys != "cat,val") || !ok {
				return fail("histogram requires numeric values with optional categories")
			}
			if cat, has := dims.Str["cat"]; keys == "cat,val" && (!has || len(cat) != len(dims.Num["val"])) {
				return fail("histogram categories must match numeric values")
			}
			for _, a := range []string{"underflow", "overflow"} {
				if v, has := HasAttr(binning, a); has && v != "auto" {
					if n, err := parseFloatStrict(v); err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
						return fail("invalid histogram bounds")
					}
				}
			}
			if ic := binning.SelectAttrValue("intervalClosed", "r"); ic != "l" && ic != "r" {
				return fail("invalid histogram interval side")
			}
			opts := binning.ChildElements()
			if len(opts) > 1 {
				return fail("select bin size or count, not both")
			}
			for _, o := range opts {
				n, err := parseFloatStrict(o.Text())
				if (!Is(o, NSCX, "binSize") && !Is(o, NSCX, "binCount")) || err != nil || math.IsInf(n, 0) || math.IsNaN(n) || n <= 0 || (Is(o, NSCX, "binCount") && (n != math.Trunc(n) || n > 4294967295)) {
					return fail("invalid histogram bin size/count")
				}
			}
			kind = "histogram"
		default:
			return fail("unsupported ChartEx layout " + layout)
		}
		out := map[string]any{}
		for k, v := range dims.Str {
			out[k] = v
		}
		for k, v := range dims.Num {
			out[k] = v
		}
		if dims.Paths != nil {
			out["category_paths"] = dims.Paths
		}
		report.Series = append(report.Series, ChartExSeries{Type: kind, DataID: did.SelectAttrValue("val", ""), Dimensions: out, dims: dims, node: s})
	}
	if report.References == 0 {
		report.LiteralSeries = len(series)
	}
	return report, nil
}

// pyG formats like Reference's format(v, '.Ng').
func pyG(v float64, prec int) string {
	if v == 0 {
		return "0"
	}
	exp := int(math.Floor(math.Log10(math.Abs(v))))
	r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'e', prec-1, 64), 64)
	if r != 0 {
		exp = int(math.Floor(math.Log10(math.Abs(r))))
	}
	if exp < -4 || exp >= prec {
		s := strconv.FormatFloat(v, 'e', prec-1, 64)
		m, e, _ := strings.Cut(s, "e")
		if strings.Contains(m, ".") {
			m = strings.TrimRight(strings.TrimRight(m, "0"), ".")
		}
		return m + "e" + e
	}
	s := strconv.FormatFloat(v, 'f', max(0, prec-1-exp), 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

type Bin struct {
	Label string
	Count float64
}

// HistogramBins bins numeric observations for a disposable ChartEx preview.
func HistogramBins(values []float64, binning *etree.Element) ([]Bin, error) {
	boundary := func(name string) (*float64, error) {
		v, ok := HasAttr(binning, name)
		if !ok || v == "auto" {
			return nil, nil
		}
		f, err := parseFloatStrict(v)
		return &f, err
	}
	low, err := boundary("underflow")
	if err != nil {
		return nil, err
	}
	high, err := boundary("overflow")
	if err != nil {
		return nil, err
	}
	if low != nil && high != nil && *low >= *high {
		return nil, errors.New("histogram overflow must exceed underflow")
	}
	start, end := slices.Min(values), slices.Max(values)
	if low != nil {
		start = *low
	}
	if high != nil {
		end = *high
	}
	var width float64
	hasWidth := false
	if s := Child(binning, NSCX, "binSize"); s != nil {
		width, err = parseFloatStrict(s.Text())
		if err != nil {
			return nil, err
		}
		hasWidth = true
	}
	w1 := 1.0
	if hasWidth {
		w1 = width
	}
	if start >= end {
		switch {
		case low != nil:
			end = start + w1
		case high != nil:
			start = end - w1
		default:
			start, end = math.Floor(start), math.Ceil(end+1)
		}
	}
	span := end - start
	tails := 0
	if low != nil {
		tails++
	}
	if high != nil {
		tails++
	}
	var regular int
	if c := Child(binning, NSCX, "binCount"); c != nil {
		n, err := parseFloatStrict(c.Text())
		if err != nil {
			return nil, err
		}
		regular = int(n) - tails
		if regular < 1 {
			return nil, errors.New("histogram bin count must include tail bins and a regular bin")
		}
		width = span / float64(regular)
	} else {
		if !hasWidth {
			dev := 0.0
			if len(values) > 1 {
				mean := 0.0
				for _, v := range values {
					mean += v
				}
				mean /= float64(len(values))
				for _, v := range values {
					dev += (v - mean) * (v - mean)
				}
				dev = math.Sqrt(dev / float64(len(values)-1))
			}
			proposed := span
			if dev != 0 {
				proposed = 3.5 * dev / math.Cbrt(float64(len(values)))
			}
			if math.IsInf(proposed, 0) || math.IsNaN(proposed) || proposed <= 0 {
				return nil, errors.New("histogram range cannot be rendered")
			}
			unit := math.Pow(10, math.Floor(math.Log10(proposed)))
			best := 1.0
			for _, n := range []float64{1, 2, 4, 5, 10} {
				if math.Abs(n-proposed/unit) < math.Abs(best-proposed/unit) {
					best = n
				}
			}
			width = best * unit
		}
		if math.IsInf(width, 0) || math.IsNaN(width) || width <= 0 || math.IsInf(span/width, 0) {
			return nil, errors.New("histogram bin width cannot be rendered")
		}
		regular = int(math.Ceil(span / width))
	}
	if math.IsInf(width, 0) || math.IsNaN(width) || width <= 0 || start+width == start {
		return nil, errors.New("histogram bins need distinct finite boundaries")
	}
	if regular < 1 || regular+tails > 1000 {
		return nil, errors.New("histogram preview supports at most 1000 bins")
	}
	if high == nil {
		end = start + float64(regular)*width
	}
	edges := make([]float64, 0, regular+1)
	for i := 0; i < regular; i++ {
		edges = append(edges, start+float64(i)*width)
	}
	edges = append(edges, end)
	for i := 1; i < len(edges); i++ {
		if edges[i-1] >= edges[i] || math.IsInf(edges[i], 0) {
			return nil, errors.New("histogram bins need increasing finite boundaries")
		}
	}
	left := binning.SelectAttrValue("intervalClosed", "r") == "l"
	counts := make([]float64, regular)
	var under, over float64
	for _, v := range values {
		switch {
		case low != nil && v <= *low:
			under++
		case high != nil && v > *high:
			over++
		default:
			var idx int
			if left {
				idx = sort.Search(len(edges), func(i int) bool { return edges[i] > v }) - 1
			} else {
				idx = sort.Search(len(edges), func(i int) bool { return edges[i] >= v }) - 1
			}
			counts[min(regular-1, max(0, idx))]++
		}
	}
	prec := 15
	for i := 1; i < len(edges); i++ {
		if pyG(edges[i-1], 15) == pyG(edges[i], 15) {
			prec = 17
		}
	}
	num := func(v float64) string { return pyG(v, prec) }
	var out []Bin
	if low != nil {
		out = append(out, Bin{"≤ " + num(*low), under})
	}
	for i, total := range counts {
		open, close := "(", "]"
		if (left || i == 0) && !(i == 0 && low != nil) {
			open = "["
		}
		if left && i < regular-1 {
			close = ")"
		}
		out = append(out, Bin{open + num(edges[i]) + ", " + num(edges[i+1]) + close, total})
	}
	if high != nil {
		out = append(out, Bin{"> " + num(*high), over})
	}
	return out, nil
}

var (
	colorCodeRe = regexp.MustCompile(`(?i)\[(?:Black|Blue|Cyan|Green|Magenta|Red|White|Yellow)\]`)
	currencyRe  = regexp.MustCompile(`\[\$([^]-]*)-[^]]*\]`)
	numFmtRe    = regexp.MustCompile(`^([^0#?]*)([0#]+(?:,[0#]+)*)(?:\.([0#]+))?([Ee][+-]0+)?([^0#?]*)$`)
	quotedRe    = regexp.MustCompile(`"([^"]*)"`)
	escapedRe   = regexp.MustCompile(`\\(.)`)
	quoteOrEsc  = regexp.MustCompile(`"[^"]*"|\\.`)
)

// ChartNumber formats numeric chart ticks for a subset of Excel formats and
// fails explicitly for unsupported ones.
func ChartNumber(value float64, code string) (string, error) {
	if strings.EqualFold(code, "general") || code == "" {
		return pyG(value, 12), nil
	}
	unsupported := errors.New("unsupported chart number format: " + code)
	sections := strings.Split(code, ";")
	section := sections[0]
	if value < 0 && len(sections) > 1 {
		section = sections[1]
	} else if value == 0 && len(sections) > 2 {
		section = sections[2]
	}
	section = colorCodeRe.ReplaceAllString(section, "")
	section = currencyRe.ReplaceAllString(section, "$1")
	if len(section) >= 2 && section[0] == '"' && section[len(section)-1] == '"' && !strings.Contains(section[1:len(section)-1], `"`) {
		return section[1 : len(section)-1], nil
	}
	m := numFmtRe.FindStringSubmatch(section)
	if m == nil {
		return "", unsupported
	}
	prefix, integer, decimal, scientific, suffix := m[1], m[2], m[3], m[4], m[5]
	literal := func(t string) (string, error) {
		t = quotedRe.ReplaceAllString(t, "$1")
		t = escapedRe.ReplaceAllString(t, "$1")
		if strings.ContainsAny(t, "[]_*@") {
			return "", unsupported
		}
		return t, nil
	}
	trimmed := strings.TrimLeft(suffix, ",")
	scaling := len(suffix) - len(trimmed)
	suffix = trimmed
	operators := strings.Count(quoteOrEsc.ReplaceAllString(prefix+suffix, ""), "%")
	var err error
	if prefix, err = literal(prefix); err != nil {
		return "", err
	}
	if suffix, err = literal(suffix); err != nil {
		return "", err
	}
	numeric := value
	if value < 0 && len(sections) > 1 {
		numeric = math.Abs(value)
	}
	numeric = numeric * math.Pow(100, float64(operators)) / math.Pow(1000, float64(scaling))
	expText := ""
	var result string
	if scientific != "" {
		if strings.Contains(integer, ",") {
			return "", unsupported
		}
		places := len(integer)
		exponent := 0
		if numeric != 0 {
			adj := int(math.Floor(math.Log10(math.Abs(numeric))))
			exponent = int(math.Floor(float64(adj)/float64(places))) * places
		}
		mantissa := numeric / math.Pow(10, float64(exponent))
		r, _ := strconv.ParseFloat(strconv.FormatFloat(mantissa, 'f', len(decimal), 64), 64)
		if math.Abs(r) >= math.Pow(10, float64(places)) {
			exponent += places
			mantissa /= math.Pow(10, float64(places))
		}
		result = strconv.FormatFloat(mantissa, 'f', len(decimal), 64)
		sign := ""
		if exponent < 0 {
			sign = "-"
		} else if scientific[1] == '+' {
			sign = "+"
		}
		e := strconv.Itoa(abs(exponent))
		for len(e) < len(scientific)-2 {
			e = "0" + e
		}
		expText = scientific[:1] + sign + e
	} else {
		result = strconv.FormatFloat(numeric, 'f', len(decimal), 64)
	}
	sign := ""
	if strings.HasPrefix(result, "-") {
		sign, result = "-", result[1:]
	}
	digits, fraction, hasPoint := strings.Cut(result, ".")
	for len(digits) < strings.Count(integer, "0") {
		digits = "0" + digits
	}
	if strings.Contains(integer, ",") {
		var groups []string
		for len(digits) > 3 {
			groups = append([]string{digits[len(digits)-3:]}, groups...)
			digits = digits[:len(digits)-3]
		}
		if digits != "" {
			groups = append([]string{digits}, groups...)
		}
		digits = strings.Join(groups, ",")
	}
	result = sign + digits
	if hasPoint {
		result += "." + fraction
	}
	optional := len(decimal) - len(strings.TrimRight(decimal, "#"))
	for i := 0; i < optional && strings.HasSuffix(result, "0"); i++ {
		result = result[:len(result)-1]
	}
	result = strings.TrimSuffix(result, ".")
	return prefix + result + expText + suffix, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// HistogramScale returns axis minimum, maximum and tick values.
func HistogramScale(axis *etree.Element, maximum float64) (float64, float64, []float64, error) {
	var scaling *etree.Element
	if axis != nil {
		scaling = Child(axis, NSCX, "valScaling")
	}
	setting := func(name string, def float64) (float64, error) {
		if scaling == nil {
			return def, nil
		}
		raw, ok := HasAttr(scaling, name)
		if !ok || raw == "auto" {
			return def, nil
		}
		return parseFloatStrict(raw)
	}
	lo, err := setting("min", 0)
	if err != nil {
		return 0, 0, nil, err
	}
	hi, err := setting("max", math.Max(1, maximum))
	if err != nil {
		return 0, 0, nil, err
	}
	if math.IsInf(lo, 0) || math.IsInf(hi, 0) || math.IsNaN(lo) || math.IsNaN(hi) || lo >= hi {
		return 0, 0, nil, errors.New("histogram axis minimum must be below its finite maximum")
	}
	step, err := setting("majorUnit", math.Max(1, math.Ceil((hi-lo)/5)))
	if err != nil {
		return 0, 0, nil, err
	}
	if math.IsInf(step, 0) || math.IsNaN(step) || step <= 0 || (hi-lo)/step > 1000 {
		return 0, 0, nil, errors.New("histogram axis needs a positive major unit and at most 1000 ticks")
	}
	if v, ok := HasAttr(scaling, "max"); scaling == nil || !ok || v == "auto" {
		hi = lo + math.Ceil((hi-lo)/step)*step
	}
	var ticks []float64
	for i := 0; i <= int(math.Floor((hi-lo)/step)); i++ {
		ticks = append(ticks, lo+float64(i)*step)
	}
	return lo, hi, ticks, nil
}

// TreeNode is one weighted node of a treemap/sunburst hierarchy.
type TreeNode struct {
	Label    string
	Value    float64
	Index    int
	Children *Tree
}

// Tree keeps child nodes in insertion order.
type Tree struct {
	Order []string
	Nodes map[string]*TreeNode
}

func NewTree() *Tree { return &Tree{Nodes: map[string]*TreeNode{}} }

func (t *Tree) Get(label string, index int) *TreeNode {
	if n := t.Nodes[label]; n != nil {
		return n
	}
	n := &TreeNode{Label: label, Index: index, Children: NewTree()}
	t.Nodes[label] = n
	t.Order = append(t.Order, label)
	return n
}

func (t *Tree) Sorted() []*TreeNode {
	var out []*TreeNode
	for _, l := range t.Order {
		out = append(out, t.Nodes[l])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out
}

type Cell struct {
	Node       *TreeNode
	X, Y, W, H float64
}

// TreemapCells packs weighted cells into rows with a golden-ratio aspect target.
func TreemapCells(t *Tree, x, y, width, height float64) []Cell {
	var pending []*TreeNode
	remaining := 0.0
	for _, n := range t.Sorted() {
		if n.Value > 0 {
			pending = append(pending, n)
			remaining += n.Value
		}
	}
	ratio := (1 + math.Sqrt(5)) / 2
	var out []Cell
	for len(pending) > 0 && remaining > 0 && width > 0 && height > 0 {
		factor := math.Max(width/height, height/width) / (remaining * ratio)
		var row []*TreeNode
		weight, best := 0.0, math.Inf(1)
		for _, n := range pending {
			trial := weight + n.Value
			scaled := trial * trial * factor
			score := math.Max(pending[0].Value/scaled, scaled/n.Value)
			if len(row) > 0 && score > best {
				break
			}
			row = append(row, n)
			weight, best = trial, score
		}
		vertical := width >= height
		extent := height * weight / remaining
		if vertical {
			extent = width * weight / remaining
		}
		pos := 0.0
		for _, n := range row {
			if vertical {
				share := height * n.Value / weight
				out = append(out, Cell{n, x, y + pos, extent, share})
				pos += share
			} else {
				share := width * n.Value / weight
				out = append(out, Cell{n, x + pos, y, share, extent})
				pos += share
			}
		}
		if vertical {
			x += extent
			width -= extent
		} else {
			y += extent
			height -= extent
		}
		remaining -= weight
		pending = pending[len(row):]
	}
	return out
}
