package office

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/efp"
)

type traceSharedGroup struct {
	master, formula, area string
	invalid               bool
}

// sharedFormula validates the group before translating bounded A1 tokens.
// It never changes package data or uses cached values to resolve references.
func (t *tracer) sharedFormula(sheet, cell, index string) (string, error) {
	if _, err := strconv.ParseUint(index, 10, 32); err != nil {
		return "", fmt.Errorf("invalid shared-formula index")
	}
	if t.shared == nil {
		t.shared = map[string]map[string]traceSharedGroup{}
	}
	groups, ok := t.shared[sheet]
	if !ok {
		groups = map[string]traceSharedGroup{}
		for ref, c := range t.wb.Sheets[sheet].Cells {
			if err := t.ctx.Err(); err != nil {
				return "", err
			}
			f := Child(c, NSS, "f")
			if f == nil || f.SelectAttrValue("t", "") != "shared" {
				continue
			}
			si := f.SelectAttrValue("si", "")
			g := groups[si]
			if f.Text() != "" {
				if g.master != "" {
					g.invalid = true
				} else {
					g.master, g.formula, g.area = ref, f.Text(), f.SelectAttrValue("ref", "")
				}
			} else if f.SelectAttrValue("ref", "") != "" {
				g.invalid = true
			}
			groups[si] = g
		}
		t.shared[sheet] = groups
	}
	g := groups[index]
	if g.invalid || g.master == "" {
		return "", fmt.Errorf("shared formula has missing or conflicting master")
	}
	area := strings.Split(g.area, ":")
	if len(area) == 1 {
		area = append(area, area[0])
	}
	if len(area) != 2 {
		return "", fmt.Errorf("invalid shared-formula range")
	}
	x1, y1, ok1 := traceCoord(area[0])
	x2, y2, ok2 := traceCoord(area[1])
	mx, my, mok := traceCoord(g.master)
	cx, cy, cok := traceCoord(cell)
	if !ok1 || !ok2 || !mok || !cok || x1 > x2 || y1 > y2 || mx < x1 || mx > x2 || my < y1 || my > y2 || cx < x1 || cx > x2 || cy < y1 || cy > y2 {
		return "", fmt.Errorf("shared-formula cell or master outside valid range")
	}
	if _, err := tokenizeTrace(g.formula); err != nil {
		return "", err
	}
	if cell == g.master {
		return g.formula, nil
	}
	return translateSharedFormula(g.formula, cx-mx, cy-my)
}

func translateSharedFormula(formula string, dx, dy int) (string, error) {
	tokens, err := tokenizeTrace(formula)
	if err != nil {
		return "", err
	}
	for i := range tokens {
		token := &tokens[i]
		if token.TType == efp.TokenTypeOperand && token.TSubType == efp.TokenSubTypeText {
			token.TValue = strings.ReplaceAll(token.TValue, `"`, `""`)
		}
		if token.TType != efp.TokenTypeOperand || token.TSubType != efp.TokenSubTypeRange {
			continue
		}
		// Table selectors are names, not A1 coordinates. Colons and exclamation
		// marks inside a header must not be split as ranges/sheet qualifiers.
		// Excel's shared-formula expansion leaves these selectors unchanged.
		if structuredRef.MatchString(token.TValue) {
			continue
		}
		prefix, ref := "", token.TValue
		if n := strings.LastIndex(ref, "!"); n >= 0 {
			prefix, ref = ref[:n], ref[n+1:]
			if strings.HasPrefix(prefix, "'") && strings.HasSuffix(prefix, "'") {
				prefix += "!"
			} else {
				prefix = "'" + strings.ReplaceAll(prefix, "'", "''") + "'!"
			}
		}
		parts := strings.Split(ref, ":")
		// The reference translator shifts each A1 endpoint of a colon chain,
		// retaining named endpoints. Dependency analysis may still report the
		// resulting operand unresolved; do not discard the entire formula.
		wholeCol, wholeRow := traceColumns.MatchString(ref), traceRows.MatchString(ref)
		for j, part := range parts {
			if x, y, ok := traceCoord(part); ok {
				absCol := strings.HasPrefix(part, "$")
				absRow := strings.Contains(strings.TrimPrefix(part, "$"), "$")
				if !absCol {
					x += dx
				}
				if !absRow {
					y += dy
				}
				if x < 1 || x > 16384 || y < 1 || y > 1048576 {
					return "", fmt.Errorf("shared reference shifts outside worksheet")
				}
				col, row := ColumnName(x), strconv.Itoa(y)
				if absCol {
					col = "$" + col
				}
				if absRow {
					row = "$" + row
				}
				parts[j] = col + row
			} else if wholeCol {
				x := ColumnIndex(strings.TrimPrefix(part, "$"))
				if !strings.HasPrefix(part, "$") {
					x += dx
				}
				if x < 1 || x > 16384 {
					return "", fmt.Errorf("shared column shifts outside worksheet")
				}
				parts[j] = ColumnName(x)
				if strings.HasPrefix(part, "$") {
					parts[j] = "$" + parts[j]
				}
			} else if wholeRow {
				y, _ := strconv.Atoi(strings.TrimPrefix(part, "$"))
				if !strings.HasPrefix(part, "$") {
					y += dy
				}
				if y < 1 || y > 1048576 {
					return "", fmt.Errorf("shared row shifts outside worksheet")
				}
				parts[j] = strconv.Itoa(y)
				if strings.HasPrefix(part, "$") {
					parts[j] = "$" + parts[j]
				}
			}
		}
		token.TValue = prefix + strings.Join(parts, ":")
	}
	p := efp.ExcelParser()
	p.Tokens.Items = tokens
	return p.Render(), nil
}
