package xlsx

import (
	"bytes"
	"fmt"
	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
	"strings"
)

// Excelize's SetSheetName does not update cell or chart formulas. Rewrite
// formula-bearing XML while retaining literal strings and external-book links.
func renameSheet(f *excelize.File, source, target string) (*excelize.File, error) {
	idx, err := f.GetSheetIndex(source)
	if err != nil {
		return nil, err
	}
	if idx < 0 {
		return nil, fmt.Errorf("source sheet not found")
	}
	source = f.GetSheetName(idx)
	other, err := f.GetSheetIndex(target)
	if err != nil {
		return nil, err
	}
	if other >= 0 && other != idx {
		return nil, fmt.Errorf("target sheet already exists")
	}
	if err = f.SetSheetName(source, target); err != nil {
		return nil, err
	}
	raw, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	p, err := opc.Read(raw.Bytes())
	if err != nil {
		return nil, err
	}
	for _, name := range p.Names() {
		if !strings.HasSuffix(name, ".xml") || !strings.HasPrefix(name, "xl/") {
			continue
		}
		d, err := p.XML(name)
		if err != nil {
			return nil, err
		}
		changed := false
		office.Iter(d.Root(), func(e *etree.Element) bool {
			switch e.Tag {
			case "f", "formula", "formula1", "formula2", "definedName", "calculatedColumnFormula", "totalsRowFormula":
				old := e.Text()
				next := renameFormulaSheet(old, source, target)
				if next != old {
					e.SetText(next)
					changed = true
				}
			}
			// Internal hyperlinks and pivot caches store the sheet name separately.
			if e.Tag == "hyperlink" {
				if a := e.SelectAttr("location"); a != nil {
					next := renameFormulaSheet(a.Value, source, target)
					if next != a.Value {
						a.Value = next
						changed = true
					}
				}
			}
			if e.Tag == "worksheetSource" {
				if a := e.SelectAttr("sheet"); a != nil && strings.EqualFold(a.Value, source) {
					a.Value = target
					changed = true
				}
			}
			return true
		})
		if changed {
			if err = p.SetXML(name, d); err != nil {
				return nil, err
			}
		}
	}
	data, err := p.Bytes()
	if err != nil {
		return nil, err
	}
	return excelize.OpenReader(bytes.NewReader(data))
}
func renameFormulaSheet(formula, source, target string) string {
	var out strings.Builder

	renameName := func(name string) (string, bool) {
		if strings.Contains(name, "[") {
			return name, false
		}
		parts := strings.Split(name, ":")
		changed := false
		for i, part := range parts {
			if strings.EqualFold(part, source) {
				parts[i] = target
				changed = true
			}
		}
		if !changed {
			return name, false
		}
		return "'" + strings.ReplaceAll(strings.Join(parts, ":"), "'", "''") + "'", true
	}
	for i := 0; i < len(formula); {
		if formula[i] == '[' {
			end := formulaBracketEnd(formula, i)
			out.WriteString(formula[i:end])
			i = end
			continue
		}
		if formula[i] == '"' {
			start := i
			i++
			for i < len(formula) {
				if formula[i] == '"' {
					i++
					if i < len(formula) && formula[i] == '"' {
						i++
						continue
					}
					break
				}
				i++
			}
			out.WriteString(formula[start:i])
			continue
		}
		if formula[i] == '\'' {
			start := i
			i++
			var name strings.Builder
			for i < len(formula) {
				if formula[i] == '\'' {
					i++
					if i < len(formula) && formula[i] == '\'' {
						name.WriteByte('\'')
						i++
						continue
					}
					break
				}
				name.WriteByte(formula[i])
				i++
			}
			if next, changed := renameName(name.String()); i < len(formula) && formula[i] == '!' && changed {
				out.WriteString(next)
			} else {
				out.WriteString(formula[start:i])
			}
			continue
		}
		if nameByte(formula[i]) {
			start := i
			for i < len(formula) && nameByte(formula[i]) {
				i++
			}

			// After ! this token is a cell coordinate, not the first sheet
			// of a 3-D qualifier. Include $ markers in absolute references.
			refStart := start
			for refStart > 0 && (nameByte(formula[refStart-1]) || formula[refStart-1] == '$') {
				refStart--
			}
			qualifiedCell := refStart > 0 && formula[refStart-1] == '!'
			if !qualifiedCell && i < len(formula) && formula[i] == ':' {
				end := i + 1
				for end < len(formula) && nameByte(formula[end]) {
					end++
				}
				if end > i+1 && end < len(formula) && formula[end] == '!' {
					i = end
				}
			}
			token := formula[start:i]
			if next, changed := renameName(token); i < len(formula) && formula[i] == '!' && (start == 0 || formula[start-1] != ']') && changed {
				out.WriteString(next)
			} else {
				out.WriteString(token)
			}
			continue
		}
		out.WriteByte(formula[i])
		i++
	}
	return out.String()
}
