package testsupport

import (
	"fmt"
	"regexp"
)

// MutationWarningFilter is one ordered filter for ZIP UserWarnings. Category is
// Warning, UserWarning, or RuntimeWarning (which does not match UserWarning).
// Patterns use Go's RE2 syntax, not arbitrary Reference regular expressions.
// Message matching is case-insensitive; module matching is case-sensitive.
// Both match at the start, as warnings.filterwarnings does. Line zero is a wildcard.
type MutationWarningFilter struct {
	Action, Message, Category, Module string
	Line                              int
}
type compiledMutationFilter struct {
	spec            MutationWarningFilter
	message, module *regexp.Regexp
}

func validMutationWarningAction(action string) bool {
	switch action {
	case "default", "module", "once", "always", "ignore", "error":
		return true
	}
	return false
}

// AddFilter preserves filterwarnings insertion, duplicate relocation, and
// suppression invalidation. Invalid filters do not modify the policy. Callers
// may share a registry across native workflows; no interpreter globals are read.
func (r *MutationWarningRegistry) AddFilter(f MutationWarningFilter, appendFilter bool) error {
	if !validMutationWarningAction(f.Action) || f.Line < 0 {
		return fmt.Errorf("invalid warning action or line")
	}
	if f.Category == "" {
		f.Category = "Warning"
	}
	switch f.Category {
	case "Warning", "UserWarning", "RuntimeWarning":
	default:
		return fmt.Errorf("unsupported warning category %q", f.Category)
	}
	c := compiledMutationFilter{spec: f}
	var err error
	if f.Message != "" {
		c.message, err = regexp.Compile("(?i:" + f.Message + ")")
		if err != nil {
			return err
		}
	}
	if f.Module != "" {
		c.module, err = regexp.Compile(f.Module)
		if err != nil {
			return err
		}
	}
	duplicate := -1
	for i, old := range r.filters {
		if old.spec == f {
			duplicate = i
			break
		}
	}
	if appendFilter {
		if duplicate < 0 {
			r.filters = append(r.filters, c)
		}
	} else {
		if duplicate >= 0 {
			r.filters = append(r.filters[:duplicate], r.filters[duplicate+1:]...)
		}
		r.filters = append([]compiledMutationFilter{c}, r.filters...)
	}
	// Even an already-present append advances the pinned CReference filter version.
	r.seen = make(map[mutationWarningKey]bool)
	return nil
}

func warningPatternMatches(p *regexp.Regexp, text string) bool {
	if p == nil {
		return true
	}
	found := p.FindStringIndex(text)
	return found != nil && found[0] == 0
}
func (r *MutationWarningRegistry) selectAction(location MutationWarningLocation, message string) string {
	for _, f := range r.filters {
		if f.spec.Category == "RuntimeWarning" || (f.spec.Line != 0 && f.spec.Line != location.Line) {
			continue
		}
		if warningPatternMatches(f.message, message) && warningPatternMatches(f.module, location.Module) {
			return f.spec.Action
		}
	}
	if r.action != "" {
		return r.action
	}
	return "default"
}
