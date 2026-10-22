package wordedit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// remapMergedBookmarks prepares the entire source body before importing any
// blocks. Reserve original source names as well as destination and allocated
// names, so a generated name cannot steal a later source bookmark's name.
func remapMergedBookmarks(d *Doc, body *etree.Element) error {
	used := map[string]bool{}
	for _, b := range all(d.MainRoot(), "bookmarkStart") {
		used[getW(b, "name")] = true
	}
	names := map[string]string{}
	starts := all(body, "bookmarkStart")
	for _, b := range starts {
		name := getW(b, "name")
		if _, ok := names[name]; ok {
			return fmt.Errorf("duplicate source bookmark name %q", name)
		}
		names[name] = name
	}
	reserved := map[string]bool{}
	for name := range used {
		reserved[name] = true
	}
	for name := range names {
		reserved[name] = true
	}
	next := 1
	for _, b := range starts {
		name := getW(b, "name")
		if !used[name] {
			continue
		}
		for {
			candidate := "PaperworksMerge" + strconv.Itoa(next)
			next++
			if !reserved[candidate] {
				names[name] = candidate
				reserved[candidate] = true
				break
			}
		}
	}
	changed := map[string]string{}
	for name, mapped := range names {
		if name != mapped {
			changed[name] = mapped
		}
	}
	if len(changed) == 0 {
		return nil
	}
	for _, b := range starts {
		setW(b, "name", names[getW(b, "name")])
	}
	for _, link := range all(body, "hyperlink") {
		// An anchor on an external relationship identifies a location in that
		// external document, not a bookmark in the appended body.
		if office.AttrV(link, nsR, "id") != "" {
			continue
		}
		if mapped, ok := changed[getW(link, "anchor")]; ok {
			setW(link, "anchor", mapped)
		}
	}
	type instruction struct {
		nodes     []*etree.Element
		separated bool
	}
	var stack []*instruction
	var walkErr error
	office.Iter(body, func(e *etree.Element) bool {
		if walkErr != nil {
			return false
		}
		switch {
		case is(e, "sectPr"):
			return false
		case is(e, "fldSimple"):
			if len(stack) > 0 && !stack[len(stack)-1].separated {
				walkErr = fmt.Errorf("unsupported nested field instruction in merged body")
				return false
			}
			var rewritten string
			rewritten, walkErr = rewriteMergedInstruction(getW(e, "instr"), changed)
			if walkErr == nil {
				setW(e, "instr", rewritten)
			}
		case is(e, "fldChar"):
			switch getW(e, "fldCharType") {
			case "begin":
				if len(stack) > 0 && !stack[len(stack)-1].separated {
					walkErr = fmt.Errorf("unsupported nested field instruction in merged body")
					return false
				}
				stack = append(stack, &instruction{})
			case "separate", "end":
				if len(stack) == 0 {
					walkErr = fmt.Errorf("unbalanced field in merged body")
					return false
				}
				cur := stack[len(stack)-1]
				if !cur.separated {
					var text strings.Builder
					for _, n := range cur.nodes {
						text.WriteString(n.Text())
					}
					rewritten, err := rewriteMergedInstruction(text.String(), changed)
					if err != nil {
						walkErr = err
						return false
					}
					// Redistribute the instruction without removing runs or their properties.
					if rewritten != text.String() {
						remaining := []rune(rewritten)
						for i, n := range cur.nodes {
							size := len([]rune(n.Text()))
							if i == len(cur.nodes)-1 || size > len(remaining) {
								size = len(remaining)
							}
							n.SetText(string(remaining[:size]))
							n.CreateAttr("xml:space", "preserve")
							remaining = remaining[size:]
						}
					}
				}
				if getW(e, "fldCharType") == "end" {
					stack = stack[:len(stack)-1]
				} else {
					cur.separated = true
				}
			}
		case is(e, "instrText"):
			if len(stack) == 0 || stack[len(stack)-1].separated {
				walkErr = fmt.Errorf("unsupported orphan field instruction in merged body")
				return false
			}
			cur := stack[len(stack)-1]
			cur.nodes = append(cur.nodes, e)
		}
		return true
	})
	if walkErr != nil {
		return walkErr
	}
	if len(stack) != 0 {
		return fmt.Errorf("unterminated field in merged body")
	}
	return nil
}

// Token offsets let us replace only the bookmark operand, retaining whitespace,
// quoting, switches and literal text exactly. Dynamic/nested operands are
// rejected by the field walker instead of guessing their eventual value.
var mergeFieldToken = regexp.MustCompile(`"[^"]*"|[^\s"]+`)

func rewriteMergedInstruction(instr string, names map[string]string) (string, error) {
	if strings.Count(instr, `"`)%2 != 0 {
		return "", fmt.Errorf("unsupported unterminated quoted field operand in merged body")
	}
	spans := mergeFieldToken.FindAllStringIndex(instr, -1)
	if len(spans) == 0 {
		return instr, nil
	}
	token := func(i int) string { return strings.Trim(instr[spans[i][0]:spans[i][1]], `"`) }
	kind := strings.ToUpper(token(0))
	target := -1
	switch kind {
	case "REF", "PAGEREF", "NOTEREF":
		if len(spans) < 2 || strings.HasPrefix(token(1), `\`) {
			return "", fmt.Errorf("unsupported %s bookmark operand in merged body", kind)
		}
		target = 1
	case "HYPERLINK":
		for i := 1; i < len(spans); i++ {
			if strings.EqualFold(token(i), `\l`) {
				if target != -1 || i+1 == len(spans) {
					return "", fmt.Errorf("unsupported HYPERLINK bookmark operand in merged body")
				}
				// A URL plus a local anchor refers to the external document.
				if i != 1 {
					if !strings.HasPrefix(token(1), `\`) {
						return instr, nil
					}
					return "", fmt.Errorf("unsupported HYPERLINK switch order in merged body")
				}
				target = i + 1
			}
		}
	default:
		// Other field grammars (e.g. TOC \\b, IF, or implicit bookmark
		// references) need their own parser. Never leave a colliding operand
		// pointing at the destination bookmark silently.
		for name := range names {
			if regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(name) + `($|[^A-Za-z0-9_])`).MatchString(instr) {
				return "", fmt.Errorf("unsupported bookmark reference in %s field in merged body", kind)
			}
		}
	}
	if target < 0 {
		return instr, nil
	}
	mapped, ok := names[token(target)]
	if !ok {
		return instr, nil
	}
	span := spans[target]
	if instr[span[0]] == '"' {
		span[0]++
		span[1]--
	}
	return instr[:span[0]] + mapped + instr[span[1]:], nil
}
