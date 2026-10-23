package pdf

import (
	"slices"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// resolvedChoiceOptions returns the nearest /Opt entry in a field's parent
// chain. Presence, rather than array length, determines precedence so an
// explicit empty local /Opt shadows options inherited from a parent.
func resolvedChoiceOptions(x *model.XRefTable, d types.Dict) types.Object {
	for depth := 0; d != nil && depth < 64; depth++ {
		if opts, present := d["Opt"]; present {
			return opts
		}
		d = dictOf(x, d["Parent"]).d
	}
	return nil
}

// choiceExportValues resolves inherited options without changing appearance labels.
// A two-element option uses its first (export) value for /V, not its display label.
func choiceExportValues(x *model.XRefTable, d types.Dict) []string {
	out := []string{}
	opts, _ := entryArray(x, resolvedChoiceOptions(x, d))
	for _, opt := range opts {
		opt, _ = deref(x, opt)
		if pair, ok := opt.(types.Array); ok && len(pair) == 2 {
			opt, _ = deref(x, pair[0])
		}
		if value, ok := textOf(opt); ok {
			out = append(out, value)
		}
	}
	return out
}

func validateChoiceValues(x *model.XRefTable, fields map[string]*field, values map[string]string, arrays map[string][]string) []ChoiceWarning {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var warnings []ChoiceWarning
	for _, name := range names {
		f := fields[name]
		if f.FT != "Ch" || (f.Ff&ffCombo != 0 && f.Ff&(1<<18) != 0) {
			continue
		}
		allowed := choiceExportValues(x, f.D)
		items, array := arrays[name]
		if !array {
			// An empty scalar clears the selection; an empty array does likewise.
			if values[name] == "" {
				continue
			}
			items = []string{values[name]}
		}
		for _, item := range items {
			if !slices.Contains(allowed, item) {
				warnings = append(warnings, ChoiceWarning{Code: "unmatched_choice", Field: name, Value: item, AllowedChoices: allowed})
			}
		}
	}
	return warnings
}
