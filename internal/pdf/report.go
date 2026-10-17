package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"os"
	"slices"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

const (
	ffNoToggleToOff = 1 << 14
	ffRadio         = 1 << 15
	ffPushbutton    = 1 << 16
	ffCombo         = 1 << 17
)

type field struct {
	Name     string
	D        types.Dict
	Num      int
	FT       string
	Ff       int
	States   []string
	Widgets  []types.Dict
	Terminal bool
}

func (f *field) button() bool { return f.FT == "Btn" && f.Ff&ffPushbutton == 0 }
func (f *field) radio() bool  { return f.button() && f.Ff&ffRadio != 0 }

// fieldTree mirrors the AcroForm hierarchy keyed by fully qualified name.
func fieldTree(x *model.XRefTable) (map[string]*field, bool, error) {
	cat, err := x.Catalog()
	if err != nil {
		return nil, false, err
	}
	acro := dictOf(x, cat["AcroForm"]).d
	if acro == nil {
		return map[string]*field{}, cat["AcroForm"] != nil, nil
	}
	out := map[string]*field{}
	seen := map[int]bool{}
	var walk func(o types.Object, prefix, ft string, ff int, depth int) error
	walk = func(o types.Object, prefix, ft string, ff int, depth int) error {
		r := dictOf(x, o)
		if r.d == nil {
			return nil
		}
		if depth > 64 || (r.num != 0 && seen[r.num]) {
			return errors.New("cyclic PDF field parents")
		}
		if r.num != 0 {
			seen[r.num] = true
		}
		if n := nameOf(x, r.d, "FT"); n != "" {
			ft = n
		}
		if _, ok := r.d["Ff"]; ok {
			ff = intOf(x, r.d, "Ff")
		}
		kids, _ := entryArray(x, r.d["Kids"])
		t, named := textOf(entry(x, r.d, "T"))
		if !named {
			return nil
		}
		name := t
		if prefix != "" {
			name = prefix + "." + t
		}
		f := &field{Name: name, D: r.d, Num: r.num, FT: ft, Ff: ff}
		out[name] = f
		hasNamedKid := false
		for _, k := range kids {
			kd := dictOf(x, k).d
			if kd == nil {
				continue
			}
			if _, ok := textOf(entry(x, kd, "T")); ok {
				hasNamedKid = true
				if err := walk(k, name, ft, ff, depth+1); err != nil {
					return err
				}
			} else {
				f.Widgets = append(f.Widgets, kd)
			}
		}
		if nameOf(x, r.d, "Subtype") == "Widget" {
			f.Widgets = append(f.Widgets, r.d)
		}
		f.Terminal = !hasNamedKid
		if ft == "Btn" {
			for _, w := range f.Widgets {
				ap := dictOf(x, w["AP"]).d
				n := dictOf(x, ap["N"])
				if _, isStream := entry(x, ap, "N").(types.StreamDict); isStream {
					continue
				}
				for k := range n.d {
					if !slices.Contains(f.States, "/"+k) {
						f.States = append(f.States, "/"+k)
					}
				}
			}
			slices.Sort(f.States)
			if !slices.Contains(f.States, "/Off") {
				f.States = append(f.States, "/Off")
			}
			if len(kids) > 0 && ff&ffNoToggleToOff != 0 {
				f.States = slices.DeleteFunc(f.States, func(s string) bool { return s == "/Off" })
			}
		}
		return nil
	}
	fields, _ := entryArray(x, acro["Fields"])
	for _, o := range fields {
		if err := walk(o, "", "", 0, 0); err != nil {
			return nil, true, err
		}
	}
	return out, true, nil
}

type Widget struct {
	Page             int     `json:"page"`
	Name             string  `json:"name"`
	Value            *string `json:"value"`
	NormalAppearance bool    `json:"normal_appearance"`
	dict             types.Dict
	owner            int
}

type FormReport struct {
	Pages       int                `json:"pages"`
	HasFormTree bool               `json:"has_form_tree"`
	Fields      map[string]*string `json:"fields"`
	Widgets     []Widget           `json:"widgets"`
	Issues      []string           `json:"issues"`
	fields      map[string]*field
}

// widgetName walks /Parent links and returns the qualified name, the first
// inherited /V and the object number of the nearest named ancestor.
func widgetName(x *model.XRefTable, w types.Dict) (string, types.Object, int, error) {
	var names []string
	var value types.Object
	owner := -1
	cur := ref{w, 0}
	seen := map[int]bool{}
	for depth := 0; cur.d != nil; depth++ {
		if depth > 64 || (cur.num != 0 && seen[cur.num]) {
			return "", nil, 0, errors.New("cyclic PDF field parents")
		}
		seen[cur.num] = cur.num != 0
		if v, ok := cur.d["V"]; ok && value == nil {
			value = v
		}
		if t, ok := textOf(entry(x, cur.d, "T")); ok {
			names = append([]string{t}, names...)
			if owner == -1 {
				owner = cur.num
			}
		}
		p, ok := cur.d["Parent"]
		if !ok {
			break
		}
		cur = dictOf(x, p)
	}
	return strings.Join(names, "."), value, owner, nil
}

func pageWidgets(ctx context.Context, c *model.Context, page int) ([]types.Dict, error) {
	d, _, err := pageDict(ctx, c, page)
	if err != nil {
		return nil, err
	}
	annots, _ := entryArray(c.XRefTable, d["Annots"])
	var out []types.Dict
	for _, a := range annots {
		ad := dictOf(c.XRefTable, a).d
		if ad != nil && nameOf(c.XRefTable, ad, "Subtype") == "Widget" {
			out = append(out, ad)
		}
	}
	return out, nil
}

func formReport(ctx context.Context, c *model.Context) (*FormReport, error) {
	x := c.XRefTable
	fields, hasForm, err := fieldTree(x)
	if err != nil {
		return nil, err
	}
	rep := &FormReport{Pages: x.PageCount, HasFormTree: hasForm, Fields: map[string]*string{}, Widgets: []Widget{}, Issues: []string{}, fields: fields}
	for name, f := range fields {
		rep.Fields[name] = display(x, f.D["V"])
	}
	for p := 1; p <= x.PageCount; p++ {
		// A permissive parser may discard a malformed annotation object (for
		// example a direct appearance stream inside a dictionary). Its absence
		// must not turn an unreadable appearance into a clean form report.
		page, _, err := pageDict(ctx, c, p)
		if err != nil {
			return nil, err
		}
		annotations, _ := entryArray(x, page["Annots"])
		for _, annotation := range annotations {
			if dictOf(x, annotation).d == nil {
				rep.Issues = append(rep.Issues, fmt.Sprintf("Annotation appearance cannot be established: unreadable annotation on page %d", p))
			}
		}
		ws, err := pageWidgets(ctx, c, p)
		if err != nil {
			return nil, err
		}
		for _, w := range ws {
			name, value, owner, err := widgetName(x, w)
			if err != nil {
				return nil, err
			}
			valueS := display(x, value)
			ap := dictOf(x, w["AP"]).d
			normal := entry(x, ap, "N")
			canonical := fields[name]
			as := display(x, w["AS"])
			expected := "/Off"
			if canonical != nil && canonical.button() {
				selected := "/Off"
				if v := display(x, canonical.D["V"]); v != nil {
					selected = *v
				}
				if nd, ok := normal.(types.Dict); ok {
					if _, has := nd[strings.TrimPrefix(selected, "/")]; has && strings.HasPrefix(selected, "/") {
						expected = selected
					}
				}
				if as == nil || *as != expected {
					rep.Issues = append(rep.Issues, "Widget/field appearance state mismatch: "+name)
				}
				if !slices.Contains(canonical.States, selected) {
					rep.Issues = append(rep.Issues, "Unknown button selection: "+name)
				}
			}
			if nd, ok := normal.(types.Dict); ok {
				state := as
				if state == nil {
					state = valueS
				}
				normal = nil
				if state != nil {
					normal = entry(x, nd, strings.TrimPrefix(*state, "/"))
				}
			}
			usable := false
			if sd, ok := normal.(types.StreamDict); ok {
				if err := sd.Decode(); err == nil {
					usable = len(sd.Content) > 0
				}
			}
			rep.Widgets = append(rep.Widgets, Widget{Page: p, Name: name, Value: valueS, NormalAppearance: usable, dict: w, owner: owner})
			if canonical == nil {
				rep.Issues = append(rep.Issues, "Widget missing from field tree: "+name)
			} else {
				allowed := []*string{display(x, canonical.D["V"])}
				if canonical.radio() {
					allowed = append(allowed, &expected)
				}
				if !slices.ContainsFunc(allowed, func(a *string) bool { return eqPtr(a, valueS) }) {
					rep.Issues = append(rep.Issues, "Widget/field value mismatch: "+name)
				}
			}
			if !usable {
				rep.Issues = append(rep.Issues, "Widget missing usable normal appearance: "+name)
			}
		}
	}
	return rep, nil
}

func eqPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Report inventories pages, the AcroForm tree and widget/field agreement.
func Report(ctx context.Context, path string) (*FormReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ReportBytes(ctx, data)
}

func ReportBytes(ctx context.Context, data []byte) (*FormReport, error) {
	// Relaxed validation deletes unresolved Fields/Annots references. Reporting
	// must inspect the original graph, not a repaired graph that hides defects.
	c, err := api.ReadContext(ctx, bytes.NewReader(data), config())
	if err == nil {
		err = c.EnsurePageCount()
	}
	if err != nil {
		return nil, err
	}
	recoverEmptyWidgetAppearances(ctx, c, data)
	return formReport(ctx, c)
}
