package office

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/efp"
)

func traceArgs(tokens []efp.Token) [][]efp.Token {
	out := [][]efp.Token{{}}
	depth := 0
	for _, v := range tokens {
		if v.TSubType == efp.TokenSubTypeStart {
			depth++
		}
		if v.TSubType == efp.TokenSubTypeStop {
			depth--
		}
		if v.TType == efp.TokenTypeArgument && depth == 0 {
			out = append(out, []efp.Token{})
		} else {
			out[len(out)-1] = append(out[len(out)-1], v)
		}
	}
	return out
}
func (t *tracer) scalar(tokens []efp.Token, sh, owner string, names []string) (any, error) {
	var clean []efp.Token
	for _, v := range tokens {
		if v.TType != efp.TokenTypeWhitespace {
			clean = append(clean, v)
		}
	}
	tokens = clean
	for i, v := range tokens {
		if v.TType == efp.TokenTypeOperatorInfix && v.TValue == "&" {
			a, e := t.scalar(tokens[:i], sh, owner, names)
			if e != nil {
				return nil, e
			}
			b, e := t.scalar(tokens[i+1:], sh, owner, names)
			if e != nil {
				return nil, e
			}
			return fmt.Sprint(a) + fmt.Sprint(b), nil
		}
	}
	if len(tokens) == 2 && tokens[0].TType == efp.TokenTypeOperatorPrefix {
		v, e := t.scalar(tokens[1:], sh, owner, names)
		if e != nil {
			return nil, e
		}
		n, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("non-numeric offset")
		}
		if tokens[0].TValue == "-" {
			n = -n
		}
		return n, nil
	}
	if len(tokens) != 1 {
		return nil, fmt.Errorf("address/offset needs literals or non-formula input cells")
	}
	v := tokens[0]
	if v.TType != efp.TokenTypeOperand {
		return nil, fmt.Errorf("unsupported address/offset expression")
	}
	switch v.TSubType {
	case efp.TokenSubTypeText:
		return v.TValue, nil
	case efp.TokenSubTypeNumber:
		return strconv.ParseFloat(v.TValue, 64)
	case efp.TokenSubTypeLogical:
		return strings.EqualFold(v.TValue, "TRUE"), nil
	case efp.TokenSubTypeRange:
		refs := t.reference(v.TValue, sh, owner, names)
		if len(refs) != 1 {
			return nil, fmt.Errorf("address/offset must resolve to one input cell")
		}
		value, e := t.wb.Value(refs[0].sheet, refs[0].cell)
		if e != nil {
			return nil, e
		}
		if value.Formula || value.Kind == "e" || value.Kind == "" {
			return nil, fmt.Errorf("Formula-valued, erroneous or blank address/offset input needs calculation")
		}
		if value.Kind == "b" {
			return value.Text == "1", nil
		}
		if n, ok := value.Number(); ok {
			return n, nil
		}
		return value.Text, nil
	}
	return nil, fmt.Errorf("unsupported address/offset value")
}
func (t *tracer) dynamic(tokens []efp.Token, sh, owner string, names []string) []traceRef {
	var out []traceRef
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		kind := strings.ToUpper(tok.TValue)
		if j := strings.LastIndex(kind, "."); j >= 0 {
			kind = kind[j+1:]
		}
		if tok.TType != efp.TokenTypeFunction || tok.TSubType != efp.TokenSubTypeStart || (kind != "INDIRECT" && kind != "OFFSET") {
			continue
		}
		j, depth := i+1, 1
		for ; j < len(tokens) && depth > 0; j++ {
			if tokens[j].TSubType == efp.TokenSubTypeStart {
				depth++
			}
			if tokens[j].TSubType == efp.TokenSubTypeStop {
				depth--
			}
		}
		if depth != 0 {
			t.issue(owner, kind, "Unbalanced dynamic reference")
			continue
		}
		refs, err := t.dynamicTarget(kind, traceArgs(tokens[i+1:j-1]), sh, owner, names)
		if err != nil {
			t.issue(owner, kind, err.Error())
		} else {
			out = append(out, refs...)
		}
		i = j - 1
	}
	return out
}

var r1c1Ref = regexp.MustCompile(`(?i)^R(\[[+-]?[0-9]+\]|[0-9]*)C(\[[+-]?[0-9]+\]|[0-9]*)$`)

func r1c1Address(address, owner string) (string, error) {
	prefix := ""
	if i := strings.LastIndex(address, "!"); i >= 0 {
		prefix = address[:i+1]
		address = address[i+1:]
	}
	origin := owner[strings.LastIndex(owner, "!")+1:]
	col, row, ok := traceCoord(origin)
	if !ok {
		return "", fmt.Errorf("invalid origin")
	}
	parts := strings.Split(address, ":")
	if len(parts) > 2 {
		return "", fmt.Errorf("invalid R1C1 address")
	}
	for i, part := range parts {
		m := r1c1Ref.FindStringSubmatch(part)
		if m == nil {
			return "", fmt.Errorf("invalid R1C1 address")
		}
		coord := func(v string, base int) int {
			if v == "" {
				return base
			}
			if strings.HasPrefix(v, "[") {
				n, _ := strconv.Atoi(strings.Trim(v, "[]"))
				return base + n
			}
			n, _ := strconv.Atoi(v)
			return n
		}
		r, c := coord(m[1], row), coord(m[2], col)
		if r < 1 || r > 1048576 || c < 1 || c > 16384 {
			return "", fmt.Errorf("R1C1 address exceeds worksheet bounds")
		}
		parts[i] = ColumnName(c) + strconv.Itoa(r)
	}
	return prefix + strings.Join(parts, ":"), nil
}
func (t *tracer) dynamicTarget(kind string, args [][]efp.Token, sh, owner string, names []string) ([]traceRef, error) {
	if kind == "INDIRECT" {
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("invalid INDIRECT arguments")
		}
		v, e := t.scalar(args[0], sh, owner, names)
		if e != nil {
			return nil, e
		}
		address, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("INDIRECT requires address text")
		}
		mode := true
		if len(args) == 2 {
			v, e = t.scalar(args[1], sh, owner, names)
			if e != nil {
				return nil, e
			}
			switch m := v.(type) {
			case bool:
				mode = m
			case float64:
				if m != 0 && m != 1 {
					return nil, fmt.Errorf("invalid INDIRECT reference style")
				}
				mode = m == 1
			default:
				return nil, fmt.Errorf("invalid INDIRECT reference style")
			}
		}
		if !mode {
			address, e = r1c1Address(address, owner)
			if e != nil {
				return nil, e
			}
		}
		tokens, e := tokenizeTrace(address)
		if e != nil || len(tokens) != 1 || tokens[0].TSubType != efp.TokenSubTypeRange {
			return nil, fmt.Errorf("invalid INDIRECT address")
		}
		// Static names still retain their context warning; direct dynamic addresses do not create name scope.
		return t.reference(tokens[0].TValue, sh, owner, names), nil
	}
	if len(args) < 3 || len(args) > 5 {
		return nil, fmt.Errorf("invalid OFFSET arguments")
	}
	if len(args[0]) != 1 || args[0][0].TSubType != efp.TokenSubTypeRange {
		return nil, fmt.Errorf("OFFSET base must be a cell, range or defined name")
	}
	refs := t.reference(args[0][0].TValue, sh, owner, names)
	if len(refs) == 0 {
		return nil, fmt.Errorf("OFFSET requires one rectangular base range")
	}
	left, top, right, bottom := 16384, 1048576, 0, 0
	seen := map[traceRef]bool{}
	dest := refs[0].sheet
	for _, r := range refs {
		if r.sheet != dest {
			return nil, fmt.Errorf("OFFSET requires one rectangular base range")
		}
		seen[r] = true
		c, row, _ := traceCoord(r.cell)
		left = min(left, c)
		right = max(right, c)
		top = min(top, row)
		bottom = max(bottom, row)
	}
	if (right-left+1)*(bottom-top+1) != len(seen) {
		return nil, fmt.Errorf("OFFSET base is not rectangular")
	}
	numbers := []int{}
	for _, arg := range args[1:] {
		v, e := t.scalar(arg, sh, owner, names)
		if e != nil {
			return nil, e
		}
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || math.Abs(n) > 1048576 {
			return nil, fmt.Errorf("OFFSET needs integer dimensions within worksheet bounds")
		}
		numbers = append(numbers, int(n))
	}
	width, height := right-left+1, bottom-top+1
	top += numbers[0]
	left += numbers[1]
	if len(numbers) > 2 {
		height = numbers[2]
	}
	if len(numbers) > 3 {
		width = numbers[3]
	}
	if min(left, top, width, height) < 1 || left+width-1 > 16384 || top+height-1 > 1048576 {
		return nil, fmt.Errorf("OFFSET exceeds worksheet bounds")
	}
	return t.reference(traceLabel(dest, ColumnName(left)+strconv.Itoa(top))+":"+ColumnName(left+width-1)+strconv.Itoa(top+height-1), sh, owner, nil), nil
}
