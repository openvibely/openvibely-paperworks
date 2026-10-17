package office

import (
	"fmt"
	"strings"

	"github.com/xuri/efp"
)

// efp represents {a,b;c,d} as synthetic ARRAY/ARRAYROW functions. Collapse a
// validated rectangular literal to an opaque operand so Render retains Excel
// braces and dependency analysis never interprets a text constant as a cell.
func collapseTraceArrays(tokens []efp.Token) ([]efp.Token, error) {
	out := make([]efp.Token, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.TType != efp.TokenTypeFunction || t.TSubType != efp.TokenSubTypeStart || t.TValue != "ARRAY" {
			out = append(out, t)
			continue
		}
		var rows []string
		width := 0
		i++
		for {
			if i >= len(tokens) || tokens[i].TValue != "ARRAYROW" || tokens[i].TSubType != efp.TokenSubTypeStart {
				return nil, fmt.Errorf("invalid array constant row")
			}
			i++
			var cells []string
			for {
				for i < len(tokens) && tokens[i].TType == efp.TokenTypeWhitespace {
					i++
				}
				sign := ""
				if i < len(tokens) && tokens[i].TType == efp.TokenTypeOperatorPrefix && (tokens[i].TValue == "-" || tokens[i].TValue == "+") {
					sign = tokens[i].TValue
					i++
				}
				if i >= len(tokens) || tokens[i].TType != efp.TokenTypeOperand {
					return nil, fmt.Errorf("array constant requires literal values")
				}
				v := tokens[i]
				value := v.TValue
				if sign != "" && v.TSubType != efp.TokenSubTypeNumber {
					return nil, fmt.Errorf("array sign requires numeric literal")
				}
				switch v.TSubType {
				case efp.TokenSubTypeNumber, efp.TokenSubTypeLogical, efp.TokenSubTypeError:
				case efp.TokenSubTypeText:
					value = `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
				default:
					return nil, fmt.Errorf("array constant cannot contain references or expressions")
				}
				cells = append(cells, sign+value)
				i++
				for i < len(tokens) && tokens[i].TType == efp.TokenTypeWhitespace {
					i++
				}
				if i < len(tokens) && tokens[i].TType == efp.TokenTypeArgument {
					i++
					continue
				}
				if i >= len(tokens) || tokens[i].TType != efp.TokenTypeFunction || tokens[i].TSubType != efp.TokenSubTypeStop {
					return nil, fmt.Errorf("invalid array constant separator")
				}
				i++
				break
			}
			if width == 0 {
				width = len(cells)
			} else if width != len(cells) {
				return nil, fmt.Errorf("array constant rows have different widths")
			}
			rows = append(rows, strings.Join(cells, ","))
			if i < len(tokens) && tokens[i].TType == efp.TokenTypeArgument {
				i++
				continue
			}
			if i >= len(tokens) || tokens[i].TType != efp.TokenTypeFunction || tokens[i].TSubType != efp.TokenSubTypeStop {
				return nil, fmt.Errorf("unterminated array constant")
			}
			break
		}
		out = append(out, efp.Token{TType: efp.TokenTypeOperand, TSubType: "ArrayConstant", TValue: "{" + strings.Join(rows, ";") + "}"})
	}
	return out, nil
}

// traceArrayErrors reads only validated, normalized literal arrays emitted by
// collapseTraceArrays. Quoted values (including doubled quotes) are data, not
// formula errors. Keep the opaque token for shared translation while exposing
// actual error operands to the same reporting path as standalone errors.
func traceArrayErrors(literal string) []string {
	var errors []string
	for i := 0; i < len(literal); {
		if literal[i] == '"' {
			i++
			for i < len(literal) {
				if literal[i] == '"' {
					i++
					if i < len(literal) && literal[i] == '"' {
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}
		if literal[i] == '#' {
			start := i
			for i < len(literal) && literal[i] != ',' && literal[i] != ';' && literal[i] != '}' {
				i++
			}
			errors = append(errors, literal[start:i])
			continue
		}
		i++
	}
	return errors
}
