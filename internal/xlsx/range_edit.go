package xlsx

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

var wholeCols = regexp.MustCompile(`^(\$?)([A-Za-z]{1,3}):(\$?)([A-Za-z]{1,3})`)
var wholeRows = regexp.MustCompile(`^(\$?)([1-9][0-9]*):(\$?)([1-9][0-9]*)`)
var a1Ref = regexp.MustCompile(`^(\$?)([A-Za-z]{1,3})(\$?)([1-9][0-9]*)`)

// TranslateFormula shifts A1 references while leaving strings, quoted sheet
// names, defined names, structured references, absolute coordinates and
// function names alone.
func TranslateFormula(s string, dx, dy int) (string, error) {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '"' || s[i] == '\'' {
			q := s[i]
			start := i
			i++
			for i < len(s) {
				if s[i] == q {
					i++
					if i < len(s) && s[i] == q {
						i++
						continue
					}
					break
				}
				i++
			}
			out.WriteString(s[start:i])
			continue
		}
		if s[i] == '[' {
			end := formulaBracketEnd(s, i)
			out.WriteString(s[i:end])
			i = end
			continue
		}
		if end := formulaSheetQualifierEnd(s, i); end > i {
			out.WriteString(s[i:end])
			i = end
			continue
		}
		if i == 0 || !nameByte(s[i-1]) {
			matched := false
			for _, axis := range []struct {
				re           *regexp.Regexp
				delta, limit int
				column       bool
			}{{wholeCols, dx, 16384, true}, {wholeRows, dy, 1048576, false}} {
				m := axis.re.FindStringSubmatch(s[i:])
				if m == nil {
					continue
				}
				end := i + len(m[0])
				if end < len(s) && nameByte(s[end]) {
					continue
				}
				// A colon can also join defined names. Outside the column grid,
				// bare identifiers such as XFE:XFF are names, not column ranges.
				if axis.column && m[1] == "" && m[3] == "" {
					_, firstErr := excelize.ColumnNameToNumber(m[2])
					_, lastErr := excelize.ColumnNameToNumber(m[4])
					if firstErr != nil || lastErr != nil {
						out.WriteString(m[0])
						i = end
						matched = true
						break
					}
				}
				vals := []string{}
				for k := 1; k <= 3; k += 2 {
					var n int
					var err error
					if axis.column {
						n, err = excelize.ColumnNameToNumber(m[k+1])
					} else {
						n, err = strconv.Atoi(m[k+1])
					}
					if err != nil {
						return "", err
					}
					if m[k] == "" {
						n += axis.delta
					}
					if n < 1 || n > axis.limit {
						return "", fmt.Errorf("translated range is out of bounds")
					}
					v := strconv.Itoa(n)
					if axis.column {
						v, _ = excelize.ColumnNumberToName(n)
					}
					vals = append(vals, m[k]+v)
				}
				out.WriteString(strings.Join(vals, ":"))
				i = end
				matched = true
				break
			}
			if matched {
				continue
			}
		}
		m := a1Ref.FindStringSubmatch(s[i:])
		if m != nil && (i == 0 || !nameByte(s[i-1])) {
			end := i + len(m[0])
			// LOG10 is both a built-in function name and a valid A1 address.
			// Allow whitespace before its argument list without removing it:
			// other references such as A1 (A1:B2) use space for intersection.
			if strings.EqualFold(m[0], "LOG10") {
				next := end
				for next < len(s) && strings.ContainsRune(" \t\r\n", rune(s[next])) {
					next++
				}
				if next < len(s) && s[next] == '(' {
					out.WriteString(s[i:next])
					i = next
					continue
				}
			}
			if end == len(s) || (!nameByte(s[end]) && s[end] != '(' && s[end] != '!' && s[end] != '[') {
				x, colErr := excelize.ColumnNameToNumber(m[2])
				y, rowErr := strconv.Atoi(m[4])
				// Check the original coordinates before applying an offset:
				// A1048577 and XFE1 are valid names outside Excel's cell grid.
				if colErr != nil || rowErr != nil || y > 1048576 {
					if m[1] != "" || m[3] != "" {
						return "", fmt.Errorf("invalid absolute cell reference")
					}
					out.WriteString(m[0])
					i = end
					continue
				}
				if m[1] == "" {
					x += dx
				}
				if m[3] == "" {
					y += dy
				}
				if x < 1 || x > 16384 || y < 1 || y > 1048576 {
					return "", fmt.Errorf("formula translation would create an out-of-bounds reference")
				}
				col, _ := excelize.ColumnNumberToName(x)
				out.WriteString(m[1] + col + m[3] + strconv.Itoa(y))
				i = end
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String(), nil
}
func nameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '\\' || c >= 128
}
func copyRange(f *excelize.File, e Edit) (*excelize.File, error) {
	a, b, err := bounds(e.Range)
	if err != nil {
		return nil, err
	}
	x, y, _ := excelize.CellNameToCoordinates(a)
	xx, yy, _ := excelize.CellNameToCoordinates(b)
	tx, ty, err := excelize.CellNameToCoordinates(e.Target)
	if err != nil {
		return nil, err
	}
	targetSheet := e.Name
	if targetSheet == "" {
		targetSheet = e.Sheet
	}
	idx, err := f.GetSheetIndex(targetSheet)
	if err != nil {
		return nil, err
	}
	if idx < 0 {
		return nil, fmt.Errorf("target sheet missing")
	}
	targetSheet = f.GetSheetName(idx)
	merges, err := f.GetMergeCells(targetSheet, true)
	if err != nil {
		return nil, err
	}
	type mergedTarget struct {
		anchor                   string
		left, top, right, bottom int
	}
	var mergedTargets []mergedTarget
	for _, merge := range merges {
		left, top, right, bottom, err := office.ParseRange(merge[0])
		if err != nil {
			return nil, err
		}
		mergedTargets = append(mergedTargets, mergedTarget{merge.GetStartAxis(), left, top, right, bottom})
	}
	type cell struct {
		target, anchor, formula, value string
		kind                           excelize.CellType
		style                          int
		rich                           []excelize.RichTextRun
	}
	var snapshot []cell
	for row := y; row <= yy; row++ {
		for col := x; col <= xx; col++ {
			source, _ := excelize.CoordinatesToCellName(col, row)
			dest, err := excelize.CoordinatesToCellName(tx+col-x, ty+row-y)
			if err != nil {
				return nil, err
			}
			formula, err := f.GetCellFormula(e.Sheet, source)
			if err != nil {
				return nil, err
			}
			if formula != "" && e.Operation != "copy-values" && e.Operation != "copy-formats" {
				formula, err = TranslateFormula(formula, tx-x, ty-y)
				if err != nil {
					return nil, err
				}
			}
			v, err := f.GetCellValue(e.Sheet, source, excelize.Options{RawCellValue: true})
			if err != nil {
				return nil, err
			}
			kind, err := f.GetCellType(e.Sheet, source)
			if err != nil {
				return nil, err
			}
			style, err := f.GetCellStyle(e.Sheet, source)
			if err != nil {
				return nil, err
			}
			rich, _ := f.GetCellRichText(e.Sheet, source)
			anchor := dest
			for _, merge := range mergedTargets {
				if dx, dy := tx+col-x, ty+row-y; dx >= merge.left && dx <= merge.right && dy >= merge.top && dy <= merge.bottom {
					anchor = merge.anchor
					break
				}
			}
			snapshot = append(snapshot, cell{dest, anchor, formula, v, kind, style, rich})
		}
	}
	errors := map[string]string{}
	for _, c := range snapshot {
		if e.Operation != "copy-formats" {
			// Multiple destinations can write the same merged anchor. Keep only
			// its final error value, matching the ordinary cell setters.
			delete(errors, c.anchor)
			if err = f.SetCellFormula(targetSheet, c.target, ""); err != nil {
				return nil, err
			}
			if c.formula != "" && e.Operation != "copy-values" {
				err = f.SetCellFormula(targetSheet, c.target, c.formula)
			} else if len(c.rich) > 0 {
				err = f.SetCellRichText(targetSheet, c.target, c.rich)
			} else {
				switch c.kind {
				// XLSX omits the type for ordinary numbers and empty cells.
				case excelize.CellTypeUnset, excelize.CellTypeNumber:
					if c.value == "" {
						err = f.SetCellValue(targetSheet, c.target, nil)
					} else {
						v, parseErr := strconv.ParseFloat(c.value, 64)
						if parseErr != nil {
							err = f.SetCellStr(targetSheet, c.target, c.value)
						} else {
							err = f.SetCellFloat(targetSheet, c.target, v, -1, 64)
						}
					}
				case excelize.CellTypeError:
					// Excelize has no error-cell setter. Prepare the destination
					// and restore its error type before the next edit runs.
					err = f.SetCellStr(targetSheet, c.target, c.value)
					errors[c.anchor] = c.value
				case excelize.CellTypeBool:
					err = f.SetCellBool(targetSheet, c.target, c.value == "1" || c.value == "TRUE")
				default:
					err = f.SetCellStr(targetSheet, c.target, c.value)
				}
			}
			if err != nil {
				return nil, err
			}
		}
		if e.Operation != "copy-values" && e.Operation != "copy-formulas" {
			if err = f.SetCellStyle(targetSheet, c.target, c.target, c.style); err != nil {
				return nil, err
			}
		}
	}
	if len(errors) == 0 {
		return f, nil
	}
	return restoreCopiedErrors(f, targetSheet, errors)
}

// Reopen the corrected package so subsequent edits observe real error values,
// including another copy, a sheet rename, or an overwrite of these cells.
func restoreCopiedErrors(f *excelize.File, sheet string, values map[string]string) (*excelize.File, error) {
	raw, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	pkg, err := opc.Read(raw.Bytes())
	if err != nil {
		return nil, err
	}
	wb, err := office.OpenWorkbook(pkg)
	if err != nil {
		return nil, err
	}
	sh := wb.Sheets[sheet]
	if sh == nil {
		return nil, fmt.Errorf("copy destination sheet missing")
	}
	for address, value := range values {
		cell := sh.Cells[address]
		if cell == nil {
			return nil, fmt.Errorf("copy destination %s missing", address)
		}
		cell.CreateAttr("t", "e")
		office.RemoveAll(cell, office.NSS, "v")
		office.RemoveAll(cell, office.NSS, "is")
		office.NewChild(cell, office.NSS, "v", nil).SetText(value)
	}
	if err = pkg.SetXML(sh.Part, sh.Doc); err != nil {
		return nil, err
	}
	data, err := pkg.Bytes()
	if err != nil {
		return nil, err
	}
	return excelize.OpenReader(bytes.NewReader(data))
}

var r1c1Ref = regexp.MustCompile(`(?i)^R(\[[+-]?[0-9]+\]|[0-9]*)C(\[[+-]?[0-9]+\]|[0-9]*)`)
var r1c1AxisRef = regexp.MustCompile(`(?i)^([RC])(\[[+-]?[0-9]+\]|[0-9]*)`)

func r1c1ReferenceEnd(s string, end int) bool {
	return end == len(s) || !nameByte(s[end]) && s[end] != '[' && s[end] != '(' && s[end] != '!'
}

func r1c1Coordinate(v string, current, limit int) (int, string, error) {
	n, prefix := current, ""
	if v != "" {
		if strings.HasPrefix(v, "[") {
			delta, err := strconv.Atoi(v[1 : len(v)-1])
			if err != nil || delta < 1-current || delta > limit-current {
				return 0, "", fmt.Errorf("R1C1 coordinate out of bounds")
			}
			n += delta
		} else {
			var err error
			n, err = strconv.Atoi(v)
			if err != nil || n < 1 || n > limit {
				return 0, "", fmt.Errorf("R1C1 coordinate out of bounds")
			}
			prefix = "$"
		}
	}
	return n, prefix, nil
}

// Single-axis R1C1 references denote entire rows/columns, not A1 cells.
// Consume both endpoints together so C1:C3 becomes $A:$C, not $A:$A:$C:$C.
func r1c1WholeAxis(s string, start, x, y int) (string, int, error) {
	m := r1c1AxisRef.FindStringSubmatch(s[start:])
	if m == nil || !r1c1ReferenceEnd(s, start+len(m[0])) {
		return "", start, nil
	}
	axis := strings.ToUpper(m[1])
	coordinate := func(v string) (string, error) {
		current, limit := y, 1048576
		if axis == "C" {
			current, limit = x, 16384
		}
		n, prefix, err := r1c1Coordinate(v, current, limit)
		if err != nil {
			return "", err
		}
		name := strconv.Itoa(n)
		if axis == "C" {
			name, _ = excelize.ColumnNumberToName(n)
		}
		return prefix + name, nil
	}
	first, err := coordinate(m[2])
	if err != nil {
		return "", start, err
	}
	end := start + len(m[0])
	separator := end
	for separator < len(s) && strings.ContainsRune(" \t\r\n", rune(s[separator])) {
		separator++
	}
	if separator == len(s) || s[separator] != ':' {
		return first + ":" + first, end, nil
	}
	next := separator + 1
	for next < len(s) && strings.ContainsRune(" \t\r\n", rune(s[next])) {
		next++
	}
	// A range may repeat its sheet/workbook qualifier on the second endpoint.
	qualifierStart := next
	if next < len(s) && s[next] == '\'' {
		next++
		for next < len(s) {
			if s[next] == '\'' {
				next++
				if next == len(s) || s[next] != '\'' {
					break
				}
			}
			next++
		}
		if next < len(s) && s[next] == '!' {
			next++
		}
	} else {
		if next < len(s) && s[next] == '[' {
			next = formulaBracketEnd(s, next)
		}
		next = formulaSheetQualifierEnd(s, next)
	}
	second := r1c1AxisRef.FindStringSubmatch(s[next:])
	if second == nil || !strings.EqualFold(second[1], axis) || !r1c1ReferenceEnd(s, next+len(second[0])) {
		return "", start, fmt.Errorf("invalid R1C1 whole-row/column range")
	}
	last, err := coordinate(second[2])
	if err != nil {
		return "", start, err
	}
	return first + ":" + s[qualifierStart:next] + last, next + len(second[0]), nil
}

func r1c1Formula(s, cell string) (string, error) {
	x, y, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '"' || s[i] == '\'' {
			q := s[i]
			start := i
			i++
			for i < len(s) {
				if s[i] == q {
					i++
					if i < len(s) && s[i] == q {
						i++
						continue
					}
					break
				}
				i++
			}
			out.WriteString(s[start:i])
			continue
		}
		if s[i] == '[' {
			end := formulaBracketEnd(s, i)
			out.WriteString(s[i:end])
			i = end
			continue
		}
		if end := formulaSheetQualifierEnd(s, i); end > i {
			out.WriteString(s[i:end])
			i = end
			continue
		}
		m := r1c1Ref.FindStringSubmatch(s[i:])
		if m != nil && (i == 0 || !nameByte(s[i-1])) {
			end := i + len(m[0])
			if r1c1ReferenceEnd(s, end) {
				row, rabs, err := r1c1Coordinate(m[1], y, 1048576)
				if err != nil {
					return "", err
				}
				col, cabs, err := r1c1Coordinate(m[2], x, 16384)
				if err != nil {
					return "", err
				}
				name, _ := excelize.ColumnNumberToName(col)
				out.WriteString(cabs + name + rabs + strconv.Itoa(row))
				i = end
				continue
			}
		}
		if i == 0 || !nameByte(s[i-1]) {
			translated, end, err := r1c1WholeAxis(s, i, x, y)
			if err != nil {
				return "", err
			}
			if end > i {
				out.WriteString(translated)
				i = end
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String(), nil
}

// formulaBracketEnd skips a structured reference (including nested selectors)
// or an external workbook name. Apostrophes escape special header characters.
func formulaBracketEnd(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if i+1 < len(s) && strings.ContainsRune("[]#@'", rune(s[i+1])) {
				i++
			}
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}

// formulaSheetQualifierEnd skips unquoted sheet names (including 3D spans)
// before either A1 or R1C1 coordinates are translated.
func formulaSheetQualifierEnd(s string, i int) int {
	if i > 0 && (nameByte(s[i-1]) || s[i-1] == '!') {
		return i
	}
	end := i
	for end < len(s) && nameByte(s[end]) {
		end++
	}
	if end > i && end < len(s) && s[end] == ':' {
		end++
		for end < len(s) && nameByte(s[end]) {
			end++
		}
	}
	if end > i && end < len(s) && s[end] == '!' {
		return end + 1
	}
	return i
}
