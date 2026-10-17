package office

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

const (
	sparklineURI  = "{05C60535-1F16-4fd2-B633-F4F36F0B64E0}"
	defaultSeries = "376092"
)

type SparklineItem struct {
	Cell  string `json:"cell"`
	Range string `json:"range"`
}

type SparklineSpec struct {
	Sheet      string                     `json:"sheet"`
	Action     string                     `json:"action"`
	Sparklines []SparklineItem            `json:"sparklines"`
	Options    map[string]json.RawMessage `json:"options"`
}

var (
	sparkBools   = []string{"markers", "high", "low", "first", "last", "negative", "displayXAxis", "displayHidden", "rightToLeft"}
	sparkEnums   = map[string][]string{"type": {"line", "column", "stacked"}, "displayEmptyCellsAs": {"gap", "zero", "span"}, "minAxisType": {"individual", "group", "custom"}, "maxAxisType": {"individual", "group", "custom"}}
	sparkNumbers = []string{"lineWeight", "manualMin", "manualMax"}
	sparkColors  = []string{"colorSeries", "colorNegative", "colorAxis", "colorMarkers", "colorFirst", "colorLast", "colorHigh", "colorLow"}
	sparkCellRe  = regexp.MustCompile(`^[A-Z]{1,3}[1-9][0-9]*$`)
	sparkColorRe = regexp.MustCompile(`^(?:[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
)

// EditSparklines sets, deletes or clears native x14 sparkline groups on one
// worksheet, leaving all other parts untouched.
func EditSparklines(p *opc.Package, spec SparklineSpec) ([]string, error) {
	action := spec.Action
	if action == "" {
		action = "set"
	}
	if action != "set" && action != "delete" && action != "clear" {
		return nil, errors.New("sparkline action must be set, delete or clear")
	}
	_, _, byName, err := WorkbookSheets(p)
	if err != nil {
		return nil, err
	}
	part, ok := byName[spec.Sheet]
	if !ok {
		return nil, errors.New("unknown sparkline worksheet")
	}
	if part == "" || !p.Has(part) {
		return nil, errors.New("worksheet must be local")
	}
	if len(spec.Sparklines) > 10000 {
		return nil, errors.New("sparkline list exceeds limits")
	}
	if action != "clear" && len(spec.Sparklines) == 0 {
		return nil, errors.New("specify sparkline target cells")
	}
	targets := map[string]bool{}
	for _, it := range spec.Sparklines {
		cell := strings.ToUpper(strings.ReplaceAll(it.Cell, "$", ""))
		if !sparkCellRe.MatchString(cell) {
			return nil, errors.New("sparkline target must be one cell")
		}
		col, row, err := SplitCell(cell)
		if err != nil || col > 16384 || row > 1048576 {
			return nil, errors.New("sparkline target outside worksheet")
		}
		if targets[cell] {
			return nil, errors.New("duplicate sparkline target")
		}
		targets[cell] = true
	}
	doc, err := p.XML(part)
	if err != nil {
		return nil, err
	}
	root := doc.Root()
	extLst := Child(root, NSS, "extLst")
	var groups *etree.Element
	for _, ext := range Children(extLst, NSS, "ext") {
		if strings.EqualFold(ext.SelectAttrValue("uri", ""), sparklineURI) {
			groups = Child(ext, NSX14, "sparklineGroups")
			break
		}
	}
	if groups != nil {
		for _, g := range Children(groups, NSX14, "sparklineGroup") {
			sparks := Child(g, NSX14, "sparklines")
			if sparks == nil {
				continue
			}
			for _, s := range Children(sparks, NSX14, "sparkline") {
				cell := strings.ToUpper(strings.ReplaceAll(Text(s, NSX14Main, "sqref"), "$", ""))
				if action == "clear" || targets[cell] {
					sparks.RemoveChild(s)
				}
			}
			if len(sparks.ChildElements()) == 0 {
				groups.RemoveChild(g)
			}
		}
	}
	if action == "set" {
		if groups == nil {
			if extLst == nil {
				extLst = root.CreateElement(qual(root.Space, "extLst"))
			}
			ext := extLst.CreateElement(qual(root.Space, "ext"))
			ext.CreateAttr("uri", sparklineURI)
			groups = ext.CreateElement("x14:sparklineGroups")
			groups.CreateAttr("xmlns:x14", NSX14)
			groups.CreateAttr("xmlns:xm", NSX14Main)
		}
		x := nsPrefix(groups, NSX14)
		xm := nsPrefix(groups, NSX14Main)
		if xm == "" {
			xm = "xm"
			groups.CreateAttr("xmlns:xm", NSX14Main)
		}
		group, err := sparklineGroup(spec, byName, x, xm)
		if err != nil {
			return nil, err
		}
		groups.AddChild(group)
	}
	if groups != nil && len(groups.ChildElements()) == 0 {
		ext := groups.Parent()
		ext.Parent().RemoveChild(ext)
		if extLst != nil && len(extLst.ChildElements()) == 0 {
			root.RemoveChild(extLst)
		}
	}
	if err := p.SetXML(part, doc); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(targets))
	for t := range targets {
		out = append(out, t)
	}
	slices.Sort(out)
	return out, nil
}

func sparklineGroup(spec SparklineSpec, sheets map[string]string, x, xm string) (*etree.Element, error) {
	g := etree.NewElement(qual(x, "sparklineGroup"))
	opts := spec.Options
	keys := make([]string, 0, len(opts))
	for k := range opts {
		known := slices.Contains(sparkBools, k) || slices.Contains(sparkNumbers, k) || slices.Contains(sparkColors, k)
		if _, ok := sparkEnums[k]; !known && !ok {
			return nil, errors.New("unsupported sparkline options: " + k)
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)
	g.CreateAttr("type", "line")
	nums := map[string]float64{}
	strs := map[string]string{}
	for _, k := range keys {
		raw := opts[k]
		switch {
		case slices.Contains(sparkColors, k):
			var s string
			if json.Unmarshal(raw, &s) != nil || !sparkColorRe.MatchString(s) {
				return nil, errors.New("use RGB or ARGB sparkline colors")
			}
			strs[k] = s
		case slices.Contains(sparkBools, k):
			var b bool
			if json.Unmarshal(raw, &b) != nil {
				return nil, errors.New("sparkline flag must be boolean")
			}
			g.CreateAttr(k, map[bool]string{true: "1", false: "0"}[b])
		case slices.Contains(sparkNumbers, k):
			var f float64
			if json.Unmarshal(raw, &f) != nil || raw[0] == 't' || raw[0] == 'f' || math.IsInf(f, 0) || math.IsNaN(f) {
				return nil, errors.New("sparkline axis/weight must be finite")
			}
			if k == "lineWeight" && f <= 0 {
				return nil, errors.New("sparkline lineWeight must be positive")
			}
			nums[k] = f
			g.CreateAttr(k, strings.TrimSpace(string(raw)))
		default:
			var s string
			if json.Unmarshal(raw, &s) != nil || !slices.Contains(sparkEnums[k], s) {
				return nil, errors.New("invalid sparkline " + k)
			}
			strs[k] = s
			g.CreateAttr(k, s)
		}
	}
	for _, axis := range []string{"Min", "Max"} {
		if strs[strings.ToLower(axis)+"AxisType"] == "custom" {
			if _, ok := nums["manual"+axis]; !ok {
				return nil, errors.New("custom sparkline axis needs manual bound")
			}
		}
	}
	lo, hasLo := nums["manualMin"]
	hi, hasHi := nums["manualMax"]
	if hasLo && hasHi && lo > hi {
		return nil, errors.New("sparkline minimum exceeds maximum")
	}
	for _, k := range sparkColors {
		v, ok := strs[k]
		if !ok && k == "colorSeries" {
			v, ok = defaultSeries, true
		}
		if !ok {
			continue
		}
		if len(v) == 6 {
			v = "FF" + v
		}
		g.CreateElement(qual(x, k)).CreateAttr("rgb", strings.ToUpper(v))
	}
	sparks := g.CreateElement(qual(x, "sparklines"))
	for _, it := range spec.Sparklines {
		sheet, rng, ok := splitSheetRef(it.Range, chartRangeRe)
		if !ok {
			return nil, errors.New("sparkline source must be a local worksheet range")
		}
		if _, ok := sheets[sheet]; !ok {
			return nil, errors.New("unknown sparkline source worksheet")
		}
		l, t, r, b, err := ParseRange(rng)
		if err != nil || l < 1 || t < 1 || r > 16384 || b > 1048576 || (r-l+1)*(b-t+1) > 100000 || (l != r && t != b) {
			return nil, errors.New("sparkline source must be a bounded row or column")
		}
		s := sparks.CreateElement(qual(x, "sparkline"))
		s.CreateElement(qual(xm, "f")).SetText(it.Range)
		s.CreateElement(qual(xm, "sqref")).SetText(strings.ToUpper(strings.ReplaceAll(it.Cell, "$", "")))
	}
	return g, nil
}
