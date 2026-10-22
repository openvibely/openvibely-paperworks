package office

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/efp"
)

type TraceNode struct {
	Sheet       string  `json:"sheet"`
	Cell        string  `json:"cell"`
	Formula     *string `json:"formula"`
	CachedValue any     `json:"cached_value"`
}
type TraceIssue struct {
	Cell      string `json:"cell"`
	Reference string `json:"reference"`
	Reason    string `json:"reason"`
}
type TraceReport struct {
	Root       string               `json:"root"`
	Nodes      map[string]TraceNode `json:"nodes"`
	Edges      [][2]string          `json:"edges"`
	Cycles     []string             `json:"cycles"`
	Unresolved []TraceIssue         `json:"unresolved"`
	Truncated  bool                 `json:"truncated"`
	Complete   bool                 `json:"complete"`
	Method     string               `json:"method"`
}
type traceRef struct{ sheet, cell string }
type tracer struct {
	dateStyles         []traceDateStyle
	ctx                context.Context
	wb                 *Workbook
	report             TraceReport
	maxNodes, maxDepth int
	lookup             map[string]string
	names              map[string]map[string]string
	visiting, expanded map[string]bool
	edges              map[[2]string]bool
	cycles             map[string]bool
	work               int
	shared             map[string]map[string]traceSharedGroup
	extents            map[string]*traceExtentIndex
}

var traceCell = regexp.MustCompile(`(?i)^\$?([A-Z]{1,3})\$?([1-9][0-9]*)$`)
var traceColumns = regexp.MustCompile(`(?i)^\$?([A-Z]{1,3}):\$?([A-Z]{1,3})$`)
var traceRows = regexp.MustCompile(`^\$?([1-9][0-9]*):\$?([1-9][0-9]*)$`)
var traceAbsolute = regexp.MustCompile(`(?i)^(\$[A-Z]+\$[1-9][0-9]*(:\$[A-Z]+\$[1-9][0-9]*)?|\$[A-Z]+:\$[A-Z]+|\$[1-9][0-9]*:\$[1-9][0-9]*)$`)

func traceLabel(sh, cell string) string { return "'" + strings.ReplaceAll(sh, "'", "''") + "'!" + cell }
func traceCoord(cell string) (int, int, bool) {
	if !traceCell.MatchString(cell) {
		return 0, 0, false
	}
	c, r, e := SplitCell(cell)
	return c, r, e == nil && c <= 16384 && r <= 1048576
}

// TraceWorkbook reports bounded static precedents, never recalculates, and never changes p.
func TraceWorkbook(ctx context.Context, p *opc.Package, sheet, cell string, maxNodes, maxDepth int) (*TraceReport, error) {
	if maxNodes < 1 || maxNodes > 10000 || maxDepth < 1 || maxDepth > 100 {
		return nil, fmt.Errorf("invalid trace limits")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wb, err := OpenWorkbook(p)
	if err != nil {
		return nil, err
	}
	t := &tracer{ctx: ctx, wb: wb, maxNodes: maxNodes, maxDepth: maxDepth, lookup: map[string]string{}, names: map[string]map[string]string{"": {}}, visiting: map[string]bool{}, expanded: map[string]bool{}, edges: map[[2]string]bool{}, cycles: map[string]bool{}}
	t.dateStyles, err = wb.traceDateStyles()
	if err != nil {
		return nil, err
	}
	t.report = TraceReport{Nodes: map[string]TraceNode{}, Edges: [][2]string{}, Cycles: []string{}, Unresolved: []TraceIssue{}, Method: "Formula precedents for saved inputs; all branches included. Whole-row/column references use saved populated extents of the referenced rows/columns. INDIRECT/OFFSET targets use literals or non-formula inputs only. Cached values may be stale. No calculation performed."}
	for _, s := range wb.Order {
		t.lookup[strings.ToLower(s)] = s
		t.names[s] = map[string]string{}
	}
	sheet = t.lookup[strings.ToLower(sheet)]
	if sheet == "" {
		return nil, fmt.Errorf("unknown trace worksheet")
	}
	if _, _, ok := traceCoord(cell); !ok {
		return nil, fmt.Errorf("trace requires one cell within Excel limits")
	}
	cell = strings.ToUpper(strings.ReplaceAll(cell, "$", ""))
	d, err := p.XML("xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	for _, n := range All(d.Root(), NSS, "definedName") {
		scope := ""
		if v := n.SelectAttrValue("localSheetId", ""); v != "" {
			i, e := strconv.Atoi(v)
			if e != nil || i < 0 || i >= len(wb.Order) {
				return nil, fmt.Errorf("invalid defined name scope")
			}
			scope = wb.Order[i]
		}
		t.names[scope][strings.ToLower(n.SelectAttrValue("name", ""))] = n.Text()
	}
	t.report.Root = traceLabel(sheet, cell)
	if err = t.visit(traceRef{sheet, cell}, 0); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	for c := range t.cycles {
		t.report.Cycles = append(t.report.Cycles, c)
	}
	sort.Strings(t.report.Cycles)
	t.report.Complete = !t.report.Truncated && len(t.report.Unresolved) == 0
	return &t.report, nil
}
func (t *tracer) issue(owner, ref, reason string) {
	if len(t.report.Unresolved) >= t.maxNodes {
		t.report.Truncated = true
		return
	}
	t.report.Unresolved = append(t.report.Unresolved, TraceIssue{owner, ref, reason})
}
func (t *tracer) visit(ref traceRef, depth int) error {
	if err := t.ctx.Err(); err != nil {
		return err
	}
	key := traceLabel(ref.sheet, ref.cell)
	if t.visiting[key] {
		t.cycles[key] = true
		return nil
	}
	if t.expanded[key] {
		return nil
	}
	if _, ok := t.report.Nodes[key]; !ok && len(t.report.Nodes) >= t.maxNodes {
		t.report.Truncated = true
		return nil
	}
	c := t.wb.Sheets[ref.sheet].Cells[ref.cell]
	v, err := t.wb.CellValueOf(c)
	if err != nil {
		return err
	}
	node := TraceNode{Sheet: ref.sheet, Cell: ref.cell}
	if v.Kind != "" {
		node.CachedValue = v.Text
		if n, ok := v.Number(); ok {
			node.CachedValue = n
		}
		if v.Kind == "b" {
			node.CachedValue = v.Text == "1"
		}
	}
	if c != nil && v.Kind == "n" {
		i, parseErr := strconv.Atoi(c.SelectAttrValue("s", "0"))
		if parseErr != nil || i < 0 || (len(t.dateStyles) > 0 && i >= len(t.dateStyles)) {
			return fmt.Errorf("invalid trace cell style")
		}
		if i < len(t.dateStyles) && t.dateStyles[i].date {
			if t.dateStyles[i].elapsed {
				n, ok := v.Number()
				if !ok {
					return fmt.Errorf("invalid duration cache")
				}
				text, err := traceDuration(n)
				if err != nil {
					return err
				}
				node.CachedValue = text
			} else if n, ok := v.Number(); ok {
				text, err := traceSerialDate(n, t.wb.Date1904)
				if err != nil {
					return err
				}
				node.CachedValue = text
			}
		}
	}
	if c != nil && c.SelectAttrValue("t", "") == "d" {
		value, err := traceISOValue(v.Text)
		if err != nil {
			return err
		}
		node.CachedValue = value
	}
	if c != nil {
		if f := Child(c, NSS, "f"); f != nil {
			// A native What-If Data Table is a formula object, not A1 formula
			// text. Match the reference reader without inventing an "=" formula
			// or claiming its inputs have been enumerated.
			if f.SelectAttrValue("t", "") == "dataTable" {
				t.report.Nodes[key] = node
				t.issue(key, "", "Unsupported formula object")
				return nil
			}
			text := f.Text()
			if f.SelectAttrValue("t", "") == "shared" {
				expanded, err := t.sharedFormula(ref.sheet, ref.cell, f.SelectAttrValue("si", ""))
				if err != nil {
					if t.ctx.Err() != nil {
						return t.ctx.Err()
					}
					t.issue(key, "", err.Error())
					text = ""
				} else {
					text = expanded
				}
			}
			formula := "=" + strings.TrimPrefix(text, "=")
			node.Formula = &formula
			if text == "" {
				t.issue(key, "", "Shared or unsupported formula requires expansion")
			}
		}
	}
	t.report.Nodes[key] = node
	if node.Formula == nil {
		t.expanded[key] = true
		return nil
	}
	if depth >= t.maxDepth {
		t.report.Truncated = true
		return nil
	}
	t.visiting[key] = true
	for _, r := range t.references(*node.Formula, ref.sheet, key, nil) {
		dest := traceLabel(r.sheet, r.cell)
		if _, ok := t.report.Nodes[dest]; !ok && len(t.report.Nodes) >= t.maxNodes {
			t.report.Truncated = true
			break
		}
		edge := [2]string{key, dest}
		if !t.edges[edge] {
			if len(t.edges) >= t.maxNodes*4 {
				t.report.Truncated = true
				break
			}
			t.edges[edge] = true
			t.report.Edges = append(t.report.Edges, edge)
		}
		if err := t.visit(r, depth+1); err != nil {
			return err
		}
	}
	delete(t.visiting, key)
	t.expanded[key] = true
	return nil
}
func tokenizeTrace(expr string) (tokens []efp.Token, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("cannot tokenize formula")
		}
	}()
	if len(expr) > 32768 {
		return nil, fmt.Errorf("formula exceeds 32768 bytes")
	}
	// efp ends a range at the first closing bracket. Mask balanced bracket
	// groups so nested structured selectors and their commas stay one operand.
	masked, brackets, maskErr := maskTraceBrackets(expr)
	if maskErr != nil {
		return nil, maskErr
	}
	p := efp.ExcelParser()
	tokens = p.Parse("=" + strings.TrimPrefix(masked, "="))
	for i := range tokens {
		if tokens[i].TSubType == efp.TokenSubTypeRange {
			for j, text := range brackets {
				tokens[i].TValue = strings.ReplaceAll(tokens[i].TValue, fmt.Sprintf("__pw_bracket_%d__", j), text)
			}
		}
	}
	if p.InString || p.InPath || p.InRange {
		return nil, fmt.Errorf("unterminated formula token")
	}
	if len(tokens) > 4096 {
		return nil, fmt.Errorf("formula exceeds 4096-token trace limit")
	}
	depth := 0
	for _, token := range tokens {
		if token.TSubType == efp.TokenSubTypeStart {
			depth++
		}
		if token.TSubType == efp.TokenSubTypeStop {
			depth--
		}
		if depth < 0 || depth > 100 {
			return nil, fmt.Errorf("unbalanced formula or expression nesting exceeds 100")
		}
		if token.TType == efp.TokenTypeUnknown {
			return nil, fmt.Errorf("unknown formula token")
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced formula")
	}
	return collapseTraceArrays(tokens)
}
func (t *tracer) references(expr, sh, owner string, names []string) []traceRef {
	if t.ctx.Err() != nil {
		return nil
	}
	t.work++
	if t.work > t.maxNodes*16 {
		t.report.Truncated = true
		return nil
	}
	if len(names) >= t.maxDepth {
		t.issue(owner, expr, "Defined-name depth limit")
		t.report.Truncated = true
		return nil
	}
	tokens, err := tokenizeTrace(expr)
	if err != nil {
		t.issue(owner, expr, err.Error())
		return nil
	}
	out := t.dynamic(tokens, sh, owner, names)
	for _, token := range tokens {
		if token.TType == efp.TokenTypeOperand && token.TSubType == "ArrayConstant" {
			for _, value := range traceArrayErrors(token.TValue) {
				t.issue(owner, value, "Formula contains an error reference/value")
			}
		}
		if token.TType == efp.TokenTypeOperand && token.TSubType == efp.TokenSubTypeError {
			t.issue(owner, token.TValue, "Formula contains an error reference/value")
		}
		if token.TType != efp.TokenTypeOperand || token.TSubType != efp.TokenSubTypeRange {
			continue
		}
		out = append(out, t.reference(token.TValue, sh, owner, names)...)
		if len(out) > t.maxNodes*4 {
			t.report.Truncated = true
			return out[:t.maxNodes*4]
		}
	}
	return out
}
func (t *tracer) reference(ref, sh, owner string, names []string) []traceRef {
	// Classify table selectors before looking for worksheet separators: a
	// literal header may itself contain '!'.
	if structuredRef.MatchString(ref) {
		if refs, ok := t.structured(ref, sh, owner, names); ok {
			return refs
		}
		t.issue(owner, ref, "Unresolved name, structured, external, or spill reference")
		return nil
	}
	dest, addr := sh, ref
	if i := strings.LastIndex(ref, "!"); i >= 0 {
		dest = strings.ReplaceAll(strings.Trim(ref[:i], "'"), "''", "'")
		addr = ref[i+1:]
	}
	if strings.Contains(dest, ":") && !strings.Contains(dest, "[") {
		ends := strings.Split(dest, ":")
		if len(ends) == 2 {
			a, b := -1, -1
			for i, s := range t.wb.Order {
				if strings.EqualFold(s, ends[0]) {
					a = i
				}
				if strings.EqualFold(s, ends[1]) {
					b = i
				}
			}
			if a >= 0 && b >= 0 {
				if a > b {
					a, b = b, a
				}
				if b-a+1 > t.maxNodes {
					t.report.Truncated = true
					t.issue(owner, ref, "3D range exceeds node budget")
					return nil
				}
				var out []traceRef
				for _, s := range t.wb.Order[a : b+1] {
					out = append(out, t.reference(traceLabel(s, addr), sh, owner, names)...)
					if len(out) > t.maxNodes*4 {
						t.report.Truncated = true
						return out[:t.maxNodes*4]
					}
					if t.ctx.Err() != nil {
						return nil
					}
				}
				return out
			}
		}
	}
	dest = t.lookup[strings.ToLower(dest)]
	l, top, r, bottom := 0, 0, 0, 0
	isRange := false
	pair := strings.Split(addr, ":")
	if len(pair) == 1 {
		if c, rw, ok := traceCoord(addr); ok {
			l, r, top, bottom = c, c, rw, rw
			isRange = true
		}
	}
	if len(pair) == 2 {
		c1, r1, ok1 := traceCoord(pair[0])
		c2, r2, ok2 := traceCoord(pair[1])
		if ok1 && ok2 {
			l, r, top, bottom = c1, c2, r1, r2
			isRange = true
		}
	}
	if m := traceColumns.FindStringSubmatch(addr); m != nil {
		l, r = ColumnIndex(m[1]), ColumnIndex(m[2])
		isRange = true
	}
	if m := traceRows.FindStringSubmatch(addr); m != nil {
		top, _ = strconv.Atoi(m[1])
		bottom, _ = strconv.Atoi(m[2])
		isRange = true
	}
	if dest == "" {
		reason := "External or missing worksheet for defined name"
		if isRange {
			reason = "External, 3D, or missing worksheet"
		}
		t.issue(owner, ref, reason)
		return nil
	}
	if !isRange {
		if strings.Contains(ref, "[") && !strings.Contains(ref, "!") {
			if refs, ok := t.structured(ref, sh, owner, names); ok {
				return refs
			}
		}
		value, ok := t.names[dest][strings.ToLower(addr)]
		if !ok {
			value, ok = t.names[""][strings.ToLower(addr)]
		}
		if !ok {
			t.issue(owner, ref, "Unresolved name, structured, external, or spill reference")
			return nil
		}
		key := dest + "!" + strings.ToLower(addr)
		for _, n := range names {
			if n == key {
				t.issue(owner, ref, "Cyclic defined name")
				return nil
			}
		}
		return t.references(value, dest, owner, append(append([]string{}, names...), key))
	}
	if len(names) > 0 && !traceAbsolute.MatchString(addr) {
		t.issue(owner, ref, "Relative defined-name reference needs application context")
	}
	if l > 16384 || r > 16384 || top > 1048576 || bottom > 1048576 || l > r || top > bottom {
		t.issue(owner, ref, "Invalid Excel range")
		return nil
	}
	if top == 0 || l == 0 {
		key := traceExtentKey{sheet: dest, columns: top == 0, first: l, last: r}
		if l == 0 {
			key.first, key.last = top, bottom
		}
		extent, ok := t.savedExtent(key)
		if !ok {
			return nil
		}
		min, max := extent[0], extent[1]
		if min == 0 {
			return nil
		}
		if top == 0 {
			top, bottom = min, max
		} else {
			l, r = min, max
		}
	}
	if (r-l+1)*(bottom-top+1) > t.maxNodes {
		t.report.Truncated = true
		t.issue(owner, ref, "Range exceeds node budget")
		return nil
	}
	var out []traceRef
	for row := top; row <= bottom; row++ {
		for col := l; col <= r; col++ {
			out = append(out, traceRef{dest, ColumnName(col) + strconv.Itoa(row)})
		}
	}
	return out
}

// Each worksheet is indexed lazily once per trace, never across workbooks or
// edits. Sparse axis maps use space proportional to populated rows and columns,
// including for worksheets with cells near Excel's maximum coordinates.
type traceExtentKey struct {
	sheet       string
	columns     bool
	first, last int
}

type traceExtentIndex struct {
	columns, rows map[int][2]int
}

func mergeTraceExtent(a, b [2]int) [2]int {
	if b[0] != 0 && (a[0] == 0 || b[0] < a[0]) {
		a[0] = b[0]
	}
	if b[1] > a[1] {
		a[1] = b[1]
	}
	return a
}

// Match Child's namespace semantics without allocating child-element slices
// for each of the three populated-cell predicates.
func tracePopulated(c *etree.Element) bool {
	if c == nil {
		return false
	}
	for _, token := range c.Child {
		if e, ok := token.(*etree.Element); ok &&
			(e.Tag == "f" || e.Tag == "v" || e.Tag == "is") && Is(e, NSS, e.Tag) {
			return true
		}
	}
	return false
}

func (t *tracer) savedExtent(key traceExtentKey) ([2]int, bool) {
	if t.ctx.Err() != nil {
		return [2]int{}, false
	}
	index := t.extents[key.sheet]
	if index == nil {
		index = &traceExtentIndex{columns: map[int][2]int{}, rows: map[int][2]int{}}
		for coord, c := range t.wb.Sheets[key.sheet].Cells {
			if t.ctx.Err() != nil {
				return [2]int{}, false
			}
			if !tracePopulated(c) {
				continue
			}
			col, row, ok := traceCoord(coord)
			if !ok {
				continue
			}
			index.columns[col] = mergeTraceExtent(index.columns[col], [2]int{row, row})
			index.rows[row] = mergeTraceExtent(index.rows[row], [2]int{col, col})
		}
		// Cancellation must not publish a partial index, even at the scan boundary.
		if t.ctx.Err() != nil {
			return [2]int{}, false
		}
		if t.extents == nil {
			t.extents = map[string]*traceExtentIndex{}
		}
		t.extents[key.sheet] = index
	}
	axis := index.rows
	if key.columns {
		axis = index.columns
	}
	extent := [2]int{}
	// Probe narrow intervals directly; for wide or invalid intervals, visit
	// only populated axes rather than a potentially million-row interval.
	if key.first >= 1 && key.last >= key.first && key.last <= 1048576 && key.last-key.first < len(axis) {
		for position := key.first; position <= key.last; position++ {
			if t.ctx.Err() != nil {
				return [2]int{}, false
			}
			extent = mergeTraceExtent(extent, axis[position])
		}
		return extent, true
	}
	for position, bounds := range axis {
		if t.ctx.Err() != nil {
			return [2]int{}, false
		}
		if position >= key.first && position <= key.last {
			extent = mergeTraceExtent(extent, bounds)
		}
	}
	return extent, true
}
