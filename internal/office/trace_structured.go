package office

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

var structuredRef = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.]*)?(\[.*\])$`)

func (t *tracer) structured(ref, sh, owner string, names []string) ([]traceRef, bool) {
	m := structuredRef.FindStringSubmatch(ref)
	if m == nil {
		return nil, false
	}
	ownerCell := owner[strings.LastIndex(owner, "!")+1:]
	oc, or, _ := traceCoord(ownerCell)
	type table struct {
		sheet                              string
		l, top, r, bottom, headers, totals int
		columns                            []string
	}
	candidates := []table{}
	for _, sheet := range t.wb.Order {
		if t.ctx.Err() != nil {
			return nil, true
		}
		s := t.wb.Sheets[sheet]
		rels, e := t.wb.Pkg.Rels(s.Part)
		if e != nil {
			t.issue(owner, ref, e.Error())
			return nil, true
		}
		for _, rel := range rels {
			if !strings.HasSuffix(rel.Type, "/table") || rel.TargetMode == "External" {
				continue
			}
			part := opc.Resolve(s.Part, rel.Target)
			d, e := t.wb.Pkg.XML(part)
			if e != nil {
				t.issue(owner, ref, e.Error())
				return nil, true
			}
			root := d.Root()
			pair := strings.Split(root.SelectAttrValue("ref", ""), ":")
			if len(pair) != 2 {
				continue
			}
			l, top, ok := traceCoord(pair[0])
			r, bottom, ok2 := traceCoord(pair[1])
			if !ok || !ok2 {
				continue
			}
			if m[1] != "" && !strings.EqualFold(m[1], root.SelectAttrValue("name", root.SelectAttrValue("displayName", ""))) {
				continue
			}
			if m[1] == "" && (sh != sheet || oc < l || oc > r || or < top || or > bottom) {
				continue
			}
			header, _ := strconv.Atoi(root.SelectAttrValue("headerRowCount", "1"))
			total, _ := strconv.Atoi(root.SelectAttrValue("totalsRowCount", "0"))
			tab := table{sheet: sheet, l: l, top: top, r: r, bottom: bottom, headers: header, totals: total}
			for _, c := range All(root, NSS, "tableColumn") {
				tab.columns = append(tab.columns, c.SelectAttrValue("name", ""))
			}
			candidates = append(candidates, tab)
		}
	}
	if len(candidates) != 1 {
		return nil, false
	}
	tab := candidates[0]
	selector := m[2]
	leaves, valid := structuredLeaves(selector)
	if !valid {
		return nil, false
	}
	mode := "#data"
	sections := 0
	cols := []string{}
	for _, raw := range leaves {
		if strings.HasPrefix(raw, "#") {
			sections++
			mode = strings.ToLower(raw)
			continue
		}
		raw = strings.TrimPrefix(raw, "@")
		if raw != "" {
			cols = append(cols, unescapeStructuredHeader(raw))
		}
	}
	if sections > 1 {
		return nil, false
	}
	if strings.HasPrefix(selector, "[@") {
		mode = "#this row"
	}
	if len(cols) > 2 || (len(cols) == 2 && !strings.Contains(selector, "]:[")) {
		return nil, false
	}
	left, right := tab.l, tab.r
	for i, c := range cols {
		index := -1
		for j, v := range tab.columns {
			if strings.EqualFold(c, v) {
				index = j
				break
			}
		}
		if index < 0 {
			return nil, false
		}
		if i == 0 {
			left = tab.l + index
		}
		right = tab.l + index
	}
	top, bottom := tab.top+tab.headers, tab.bottom-tab.totals
	switch mode {
	case "#data":
	case "#all":
		top, bottom = tab.top, tab.bottom
	case "#headers":
		top, bottom = tab.top, tab.top+tab.headers-1
	case "#totals":
		top, bottom = tab.bottom-tab.totals+1, tab.bottom
	case "#this row":
		if sh != tab.sheet || or < top || or > bottom {
			t.issue(owner, ref, "This-row reference outside table data")
			return nil, true
		}
		top, bottom = or, or
	default:
		return nil, false
	}
	if top > bottom {
		return nil, true
	}
	if left > right {
		return nil, false
	}
	return t.reference(traceLabel(tab.sheet, ColumnName(left)+strconv.Itoa(top))+":"+ColumnName(right)+strconv.Itoa(bottom), sh, owner, nil), true
}

// Preserve quoted strings and sheet names; brackets inside them are not selectors.
func maskTraceBrackets(expr string) (string, []string, error) {
	if strings.Contains(expr, "__pw_bracket_") {
		return "", nil, fmt.Errorf("reserved trace tokenizer identifier")
	}
	var out strings.Builder
	var groups []string
	for i := 0; i < len(expr); {
		if expr[i] == '"' || expr[i] == '\'' {
			quote := expr[i]
			start := i
			i++
			for i < len(expr) {
				if expr[i] == quote {
					if i+1 < len(expr) && expr[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			if quote == '\'' && i < len(expr) && expr[i] == '!' {
				// Preserve the spelling efp otherwise strips from worksheet names.
				fmt.Fprintf(&out, "__pw_bracket_%d__", len(groups))
				groups = append(groups, expr[start:i])
			} else {
				out.WriteString(expr[start:i])
			}
			continue
		}
		if expr[i] != '[' {
			out.WriteByte(expr[i])
			i++
			continue
		}
		start, depth := i, 0
		for i < len(expr) {
			if structuredEscape(expr, i) {
				i += 2
				continue
			}
			if expr[i] == '[' {
				depth++
			}
			if expr[i] == ']' {
				depth--
			}
			i++
			if depth == 0 {
				break
			}
		}
		if depth != 0 {
			return "", nil, fmt.Errorf("unbalanced structured reference")
		}
		fmt.Fprintf(&out, "__pw_bracket_%d__", len(groups))
		groups = append(groups, expr[start:i])
	}
	return out.String(), groups, nil
}

// Apostrophes escape only Excel's five special header characters. Escaped
// brackets are data, never nesting delimiters or end-of-operand markers.
func structuredEscape(s string, i int) bool {
	return s[i] == '\'' && i+1 < len(s) && strings.ContainsRune("[]#'@", rune(s[i+1]))
}
func unescapeStructuredHeader(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if structuredEscape(s, i) {
			i++
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

// Extract innermost selectors without interpreting escaped delimiters. Retain
// escapes until section/current-row markers have been distinguished from names.
func structuredLeaves(s string) ([]string, bool) {
	type frame struct {
		start int
		child bool
	}
	stack := []frame{}
	leaves := []string{}
	for i := 0; i < len(s); i++ {
		if structuredEscape(s, i) {
			i++
			continue
		}
		switch s[i] {
		case '[':
			if len(stack) > 0 {
				stack[len(stack)-1].child = true
			}
			stack = append(stack, frame{start: i + 1})
		case ']':
			if len(stack) == 0 {
				return nil, false
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !f.child {
				leaves = append(leaves, s[f.start:i])
			}
		}
	}
	return leaves, len(stack) == 0 && len(leaves) > 0
}
