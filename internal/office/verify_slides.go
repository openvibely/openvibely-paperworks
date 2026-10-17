package office

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

type TableSum struct {
	Slide     int         `json:"slide"`
	Table     int         `json:"table"`
	Inputs    [][2]int    `json:"inputs"`
	Total     [2]int      `json:"total"`
	Tolerance json.Number `json:"tolerance"`
}

type SlideRequirements struct {
	Slides            *int       `json:"slides"`
	SizeEMU           []int64    `json:"size_emu"`
	NativeTables      []int      `json:"native_tables"`
	NativeCharts      []int      `json:"native_charts"`
	Fonts             []string   `json:"fonts"`
	TableSums         []TableSum `json:"table_sums"`
	TargetApplication *string    `json:"target_application"`
	RenderFonts       bool       `json:"render_fonts"`
}

// SlideCheck is the static part of verify-slides; rendered font checks are
// layered on by the caller.
type SlideCheck struct {
	Slides          int
	TableOwners     []int
	ChartOwners     []int
	ExplicitFonts   []string
	FontDefinitions any
	ChartData       []any
	Headings        []map[string]any
	LayoutReview    []map[string]any
}

func tableCellText(tc *etree.Element) string {
	var parts []string
	for _, par := range All(tc, NSA, "p") {
		parts = append(parts, paraText(par))
	}
	return strings.Join(parts, "\n")
}

var tableCurrencyRe = regexp.MustCompile(`^(-?)[$€£]`)

func tableNumber(tbl *etree.Element, cell [2]int) (*big.Rat, error) {
	row, col := cell[0], cell[1]
	if row < 0 || col < 0 {
		return nil, errors.New("negative cell index")
	}
	rows := Children(tbl, NSA, "tr")
	if row >= len(rows) {
		return nil, errors.New("row index out of range")
	}
	cells := Children(rows[row], NSA, "tc")
	if col >= len(cells) {
		return nil, errors.New("column index out of range")
	}
	text := strings.ReplaceAll(strings.TrimSpace(tableCellText(cells[col])), ",", "")
	if strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
		text = "-" + text[1:len(text)-1]
	}
	text = tableCurrencyRe.ReplaceAllString(text, "$1")
	percent := strings.HasSuffix(text, "%")
	text = strings.TrimSuffix(text, "%")
	v, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, fmt.Errorf("invalid number %q", text)
	}
	if percent {
		v.Quo(v, big.NewRat(100, 1))
	}
	return v, nil
}

func checkTableSum(c TableSum, tables map[int][]*etree.Element) (string, error) {
	if c.Table < 1 {
		return "", errors.New("table index starts at 1")
	}
	list := tables[c.Slide]
	if c.Table > len(list) {
		return "", errors.New("table not found")
	}
	tbl := list[c.Table-1]
	sum := new(big.Rat)
	for _, cell := range c.Inputs {
		v, err := tableNumber(tbl, cell)
		if err != nil {
			return "", err
		}
		sum.Add(sum, v)
	}
	total, err := tableNumber(tbl, c.Total)
	if err != nil {
		return "", err
	}
	tol := new(big.Rat)
	if c.Tolerance != "" {
		if _, ok := tol.SetString(string(c.Tolerance)); !ok {
			return "", errors.New("values/tolerance must be finite and tolerance nonnegative")
		}
	}
	if tol.Sign() < 0 {
		return "", errors.New("values/tolerance must be finite and tolerance nonnegative")
	}
	diff := new(big.Rat).Sub(sum, total)
	if diff.Abs(diff).Cmp(tol) > 0 {
		return fmt.Sprintf("Slide %d table %d total %s differs from %s", c.Slide, c.Table, total.FloatString(2), sum.FloatString(2)), nil
	}
	return "", nil
}

// VerifyPresentation checks explicit requirements; it never invents
// required objects or fonts.
func VerifyPresentation(p *opc.Package, report Report, req SlideRequirements) (*SlideCheck, error) {
	issues, _ := report["issues"].([]string)
	failures := append([]string{}, issues...)
	slides, err := PresentationSlides(p)
	if err != nil {
		return nil, err
	}
	pres, err := p.XML("ppt/presentation.xml")
	if err != nil {
		return nil, err
	}
	if req.Slides != nil && len(slides) != *req.Slides {
		failures = append(failures, "Slide count does not match requested total (including cover)")
	}
	if req.SizeEMU != nil {
		sz := Child(pres.Root(), NSP, "sldSz")
		got := []int64{int64(emuAttr(sz, "cx", 0)), int64(emuAttr(sz, "cy", 0))}
		if !slices.Equal(got, req.SizeEMU) {
			failures = append(failures, "Slide size does not match requested template")
		}
	}
	out := &SlideCheck{Slides: len(slides), FontDefinitions: report["font_definitions"]}
	tables := map[int][]*etree.Element{}
	fonts := map[string]bool{}
	for i, part := range slides {
		number := i + 1
		doc, err := p.XML(part)
		if err != nil {
			return nil, err
		}
		root := doc.Root()
		f, h, l := PresentationTextChecks(p, part, root, number)
		failures = append(failures, f...)
		out.Headings = append(out.Headings, h...)
		out.LayoutReview = append(out.LayoutReview, l...)
		tables[number] = All(root, NSA, "tbl")
		if len(tables[number]) > 0 {
			out.TableOwners = append(out.TableOwners, number)
		}
		if len(All(root, NSC, "chart")) > 0 || len(All(root, NSCX, "chart")) > 0 {
			out.ChartOwners = append(out.ChartOwners, number)
		}
		for _, sp := range All(root, NSP, "txBody") {
			for _, par := range Children(sp, NSA, "p") {
				for _, r := range paragraphRuns(par) {
					if r.family != "" {
						fonts[r.family] = true
					}
				}
			}
		}
	}
	for _, n := range req.NativeTables {
		if len(tables[n]) == 0 {
			failures = append(failures, fmt.Sprintf("Slide %d is missing a native table", n))
		}
	}
	for _, n := range req.NativeCharts {
		if !slices.Contains(out.ChartOwners, n) {
			failures = append(failures, fmt.Sprintf("Slide %d is missing a native chart", n))
		}
	}
	for f := range fonts {
		out.ExplicitFonts = append(out.ExplicitFonts, f)
	}
	slices.Sort(out.ExplicitFonts)
	if req.Fonts != nil {
		var unexpected []string
		for _, f := range out.ExplicitFonts {
			if !slices.Contains(req.Fonts, f) {
				unexpected = append(unexpected, f)
			}
		}
		if len(unexpected) > 0 {
			failures = append(failures, "Unexpected explicit fonts: "+strings.Join(unexpected, ", "))
		}
	}
	for _, c := range req.TableSums {
		msg, err := checkTableSum(c, tables)
		if err != nil {
			failures = append(failures, "Invalid table arithmetic contract: "+err.Error())
		} else if msg != "" {
			failures = append(failures, msg)
		}
	}
	if len(failures) > 0 {
		return nil, errors.New(strings.Join(failures, "; "))
	}
	target := ""
	if req.TargetApplication != nil {
		target = *req.TargetApplication
		if target != "powerpoint" {
			return nil, errors.New("target_application must be powerpoint when explicitly requested")
		}
	}
	if out.ChartData, err = VerifyChartData(p, target); err != nil {
		return nil, err
	}
	return out, nil
}
