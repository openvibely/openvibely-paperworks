package pdf

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	pdffont "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/primitives"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// FillOptions controls optional validation. The default preserves arbitrary choice values.
type FillOptions struct {
	StrictChoices bool
}

// ChoiceWarning identifies a value absent from a choice field's export values.
type ChoiceWarning struct {
	Code           string   `json:"code"`
	Field          string   `json:"field"`
	Value          string   `json:"value"`
	AllowedChoices []string `json:"allowed_choices"`
}

func (w ChoiceWarning) Error() string {
	return fmt.Sprintf("invalid choice for %s: %q (allowed: %q)", w.Field, w.Value, w.AllowedChoices)
}

type FillResult struct {
	Warnings          []ChoiceWarning `json:"warnings,omitempty"`
	Output            string          `json:"output"`
	FieldsFilled      []string        `json:"fields_filled"`
	Flattened         bool            `json:"flattened"`
	VisuallyInspected bool            `json:"visually_inspected"`
}

func sameFile(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	if aa == bb {
		return true
	}
	sa, e1 := os.Stat(a)
	sb, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(sa, sb)
}

// Fill sets field values by fully qualified name, validates the saved form
// and optionally flattens every widget into page content.
func Fill(ctx context.Context, path, dest string, values map[string]string, flatten bool, options ...FillOptions) (*FillResult, error) {
	return fill(ctx, path, dest, values, nil, flatten, options...)
}

// FillValues accepts strings and arrays of strings for multi-selection list boxes.
func FillValues(ctx context.Context, path, dest string, values map[string]any, flatten bool, options ...FillOptions) (*FillResult, error) {
	scalar := map[string]string{}
	arrays := map[string][]string{}
	for name, value := range values {
		switch v := value.(type) {
		case string:
			scalar[name] = v
		case []string:
			arrays[name] = v
			scalar[name] = "[" + strings.Join(v, ", ") + "]"
		case []any:
			items := make([]string, len(v))
			for i, item := range v {
				text, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("selection for %s must contain strings", name)
				}
				items[i] = text
			}
			arrays[name] = items
			scalar[name] = "[" + strings.Join(items, ", ") + "]"
		default:
			return nil, fmt.Errorf("value for %s must be a string or string array", name)
		}
	}
	return fill(ctx, path, dest, scalar, arrays, flatten, options...)
}

func fill(ctx context.Context, path, dest string, values map[string]string, arrays map[string][]string, flatten bool, options ...FillOptions) (*FillResult, error) {
	if arrays == nil {
		arrays = map[string][]string{}
	}
	if _, err := os.Lstat(dest); err == nil || sameFile(path, dest) {
		return nil, errors.New("output must be a new PDF file")
	}
	c, _, err := LoadFile(ctx, path)
	if err != nil {
		return nil, err
	}
	if c.Encrypt != nil {
		return nil, errors.New("decrypt an authorized copy before filling")
	}
	x := c.XRefTable
	fields, _, err := fieldTree(x)
	if err != nil {
		return nil, err
	}
	cat, err := x.Catalog()
	if err != nil {
		return nil, err
	}
	acro := dictOf(x, cat["AcroForm"]).d
	if acro == nil {
		acro = types.Dict{}
	}
	roots, _ := entryArray(x, acro["Fields"])
	reattached := false
	owners := map[string]int{}
	for p := 1; p <= x.PageCount; p++ {
		page, _, err := pageDict(ctx, c, p)
		if err != nil {
			return nil, err
		}
		annots, _ := entryArray(x, page["Annots"])
		for i, object := range annots {
			r := dictOf(x, object)
			w := r.d
			if w == nil || nameOf(x, w, "Subtype") != "Widget" {
				continue
			}
			if r.num == 0 {
				ir, err := x.IndRefForNewObject(w)
				if err != nil {
					return nil, err
				}
				object, annots[i], r.num = *ir, *ir, ir.ObjectNumber.Value()
			}
			name, _, owner, err := widgetName(x, w)
			if err != nil {
				return nil, err
			}
			if owner == -1 {
				return nil, errors.New("unnamed form widget requires repair before filling")
			}
			if owner == 0 {
				owner = r.num
			}
			if prev, ok := owners[name]; ok && prev != owner {
				return nil, errors.New("ambiguous duplicate PDF field: " + name)
			}
			owners[name] = owner
			if f := fields[name]; f != nil && f.Num != 0 && owner != 0 && f.Num != owner {
				return nil, errors.New("widget and canonical field disagree: " + name)
			}
			// pypdf reattach_fields adds orphan widgets carrying their own /FT.
			if fields[name] == nil && nameOf(x, w, "FT") != "" {
				already := false
				for _, root := range roots {
					if dictOf(x, root).num == r.num {
						already = true
						break
					}
				}
				if !already {
					roots = append(roots, object)
					reattached = true
				}
			}
		}
	}
	if reattached {
		acro["Fields"] = roots
		cat["AcroForm"] = acro
		fields, _, err = fieldTree(x)
		if err != nil {
			return nil, err
		}
	}
	var unknown []string
	for k := range values {
		if fields[k] == nil {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, errors.New("unknown PDF fields: " + strings.Join(unknown, ", "))
	}
	for _, f := range fields {
		if f.FT == "Sig" {
			return nil, errors.New("signature forms require an explicit signature-preserving workflow")
		}
	}
	for name := range arrays {
		f := fields[name]
		if f.FT != "Ch" || f.Ff&ffCombo != 0 || f.Ff&(1<<21) == 0 {
			return nil, fmt.Errorf("array value requires a multi-selection list box: %s", name)
		}
	}
	for name, v := range values {
		if f := fields[name]; f.FT == "Btn" && !slices.Contains(f.States, v) {
			return nil, fmt.Errorf("invalid button state for %s: %s (allowed: %s)", name, v, strings.Join(f.States, ", "))
		}
	}
	filled := map[string]string{}
	if flatten {
		for name, f := range fields {
			if !f.Terminal || len(f.Widgets) == 0 {
				continue
			}
			if _, supplied := values[name]; !supplied && f.FT == "Ch" {
				if items, ok := entryArray(x, f.D["V"]); ok {
					selection := []string{}
					for _, item := range items {
						text, ok := textOf(item)
						if !ok {
							return nil, fmt.Errorf("invalid array selection for %s", name)
						}
						selection = append(selection, text)
					}
					arrays[name] = selection
				}
			}
			if v := display(x, f.D["V"]); v != nil {
				filled[name] = *v
			} else if f.FT == "Btn" {
				filled[name] = "/Off"
			} else {
				filled[name] = ""
			}
		}
	}
	for k, v := range values {
		filled[k] = v
	}
	warnings := validateChoiceValues(x, fields, filled, arrays)
	if len(options) > 0 && options[0].StrictChoices && len(warnings) > 0 {
		return nil, warnings[0]
	}
	if err := applyValues(ctx, c, fields, filled, arrays); err != nil {
		return nil, err
	}
	// Saved appearances are authoritative after filling. Leaving an inherited
	// regeneration request enabled lets viewers replace the appearance we just
	// generated, unlike the reference's auto_regenerate=false workflow.
	cat, err = x.Catalog()
	if err != nil {
		return nil, err
	}
	if form := dictOf(x, cat["AcroForm"]).d; form != nil {
		form["NeedAppearances"] = types.Boolean(false)
	}
	// Preserve both document information and its absence through both writes.
	withoutInfo := c.Info == nil
	data, err := serializeFilled(ctx, c, withoutInfo)
	if err != nil {
		return nil, err
	}
	rep, err := ReportBytes(ctx, data)
	if err != nil {
		return nil, err
	}
	for name, v := range filled {
		if got := rep.Fields[name]; got == nil || *got != v {
			if !(v == "" && got == nil) {
				return nil, fmt.Errorf("saved PDF values do not match requested values (%s)", name)
			}
		}
	}
	if len(rep.Issues) > 0 {
		return nil, fmt.Errorf("filled PDF failed form validation: %s", strings.Join(rep.Issues, "; "))
	}
	if flatten {
		c, err = Load(ctx, data)
		if err != nil {
			return nil, err
		}
		if err := flattenForm(ctx, c); err != nil {
			return nil, err
		}
		if data, err = serializeFilled(ctx, c, withoutInfo); err != nil {
			return nil, err
		}
		rep, err = ReportBytes(ctx, data)
		if err != nil {
			return nil, err
		}
		if len(rep.Widgets) > 0 || rep.HasFormTree {
			return nil, errors.New("flattening left editable fields")
		}
	}
	if err := opc.WriteNew(dest, data); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)
	return &FillResult{Output: dest, FieldsFilled: names, Flattened: flatten, Warnings: warnings}, nil
}

func fillFonts(ctx context.Context, x *model.XRefTable) error {
	d, err := primitives.FormFontResDict(x)
	if err != nil || d == nil {
		return err
	}
	for k, v := range d {
		ir, ok := v.(types.IndirectRef)
		if !ok {
			continue
		}
		name, _, _, err := primitives.FormFontDetails(ctx, x, ir)
		if err != nil {
			continue
		}
		if ok, _ := x.FontRepository().SupportedFont(ctx, name); ok {
			x.FillFonts[k] = ir
		}
	}
	return nil
}

func inheritedString(x *model.XRefTable, d types.Dict, key string) *string {
	for i := 0; d != nil && i < 64; i++ {
		if s, ok := textOf(entry(x, d, key)); ok {
			return &s
		}
		d = dictOf(x, d["Parent"]).d
	}
	return nil
}

func applyValues(ctx context.Context, c *model.Context, fields map[string]*field, values map[string]string, selections ...map[string][]string) error {
	x := c.XRefTable
	if err := fillFonts(ctx, x); err != nil {
		return err
	}
	fonts := map[string]types.IndirectRef{}
	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		f, v := fields[name], values[name]
		da := inheritedString(x, f.D, "DA")
		switch f.FT {
		case "Btn":
			if f.Ff&ffPushbutton != 0 {
				continue
			}
			state := strings.TrimPrefix(v, "/")
			f.D["V"] = types.Name(state)
			for _, w := range f.Widgets {
				nd := dictOf(x, dictOf(x, w["AP"]).d["N"]).d
				widgetState := "Off"
				if _, ok := nd[state]; ok {
					widgetState = state
				}
				w["AS"] = types.Name(widgetState)
				// pypdf writes the selected value on the radio group and each
				// child's own selected/off value. Preserve that distinction so
				// readers inspecting widget-local /V do not inherit the group value.
				w["V"] = types.Name(widgetState)
			}
		case "Tx":
			s, err := types.EscapedUTF16String(v)
			if err != nil {
				return err
			}
			f.D["V"] = types.StringLiteral(*s)
			multi, comb := f.Ff&(1<<12) != 0, f.Ff&(1<<24) != 0
			maxLen := intOf(x, f.D, "MaxLen")
			for _, w := range f.Widgets {
				if err := primitives.EnsureTextFieldAP(ctx, c, w, v, multi, comb, maxLen, da, fonts); err != nil {
					return fmt.Errorf("text appearance for %s: %w", name, err)
				}
				if err := alignSingleLineAppearance(x, w, f, v, da); err != nil {
					return err
				}
				if err := restoreWidgetDecoration(x, w); err != nil {
					return err
				}
			}
		case "Ch":
			opts := choiceOptions(x, f.D)
			s, err := types.EscapedUTF16String(v)
			if err != nil {
				return err
			}
			ind := types.Array{}
			if i := slices.Index(opts, v); i >= 0 {
				ind = append(ind, types.Integer(i))
			}
			f.D["V"] = types.StringLiteral(*s)
			// Choices store V without I, matching the reference. Keep indices
			// locally for appearance generation; array selections retain their
			// requested ordering in V.
			delete(f.D, "I")
			if len(selections) > 0 {
				if selected, ok := selections[0][name]; ok {
					array := types.Array{}
					ind = types.Array{}
					for _, item := range selected {
						escaped, err := types.EscapedUTF16String(item)
						if err != nil {
							return err
						}
						array = append(array, types.StringLiteral(*escaped))
						if i := slices.Index(opts, item); i >= 0 {
							ind = append(ind, types.Integer(i))
						}
					}
					f.D["V"] = array
					// The reference concatenates selections for appearance matching,
					// even when a label spans adjacent selected values. V stays typed.
					v = strings.Join(selected, "")
				}
			}
			for _, w := range f.Widgets {
				var err error
				if f.Ff&ffCombo != 0 {
					err = primitives.EnsureComboBoxAP(ctx, c, w, v, da, fonts)
				} else {
					err = primitives.EnsureListBoxAP(ctx, c, w, opts, ind, da, fonts)
				}
				if err != nil {
					return fmt.Errorf("choice appearance for %s: %w", name, err)
				}
				if err := alignSingleLineAppearance(x, w, f, v, da); err != nil {
					return err
				}
				if err := restoreWidgetDecoration(x, w); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported field type %q for %s", f.FT, name)
		}
	}
	return pdffont.UpdateUserfonts(ctx, x, fonts)
}

func choiceOptions(x *model.XRefTable, d types.Dict) []string {
	a, _ := entryArray(x, entry(x, d, "Opt"))
	var out []string
	for _, o := range a {
		o, _ = deref(x, o)
		if pair, ok := o.(types.Array); ok && len(pair) == 2 {
			o, _ = deref(x, pair[1])
		}
		if s, ok := textOf(o); ok {
			out = append(out, s)
		}
	}
	return out
}

// appearanceTransform maps the transformed /BBox of a form XObject onto the
// widget /Rect, matching viewers' annotation appearance algorithm.
func appearanceTransform(bbox, matrix, rect []float64) ([6]float64, error) {
	var t [6]float64
	if len(bbox) != 4 || len(matrix) != 6 || len(rect) != 4 {
		return t, errors.New("invalid PDF widget appearance geometry")
	}
	for _, v := range slices.Concat(bbox, matrix, rect) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return t, errors.New("invalid PDF widget appearance geometry")
		}
	}
	a, b, c, d, e, f := matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5]
	left, right, bottom, top := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, px := range []float64{bbox[0], bbox[2]} {
		for _, py := range []float64{bbox[1], bbox[3]} {
			x, y := a*px+c*py+e, b*px+d*py+f
			left, right = math.Min(left, x), math.Max(right, x)
			bottom, top = math.Min(bottom, y), math.Max(top, y)
		}
	}
	r := []float64{math.Min(rect[0], rect[2]), math.Min(rect[1], rect[3]), math.Max(rect[0], rect[2]), math.Max(rect[1], rect[3])}
	if right <= left || top <= bottom || r[2] <= r[0] || r[3] <= r[1] {
		return t, errors.New("degenerate PDF widget appearance geometry")
	}
	sx, sy := (r[2]-r[0])/(right-left), (r[3]-r[1])/(top-bottom)
	return [6]float64{sx, 0, 0, sy, r[0] - sx*left, r[1] - sy*bottom}, nil
}

func flattenForm(ctx context.Context, c *model.Context) error {
	x := c.XRefTable
	for p := 1; p <= x.PageCount; p++ {
		pd, inh, err := pageDict(ctx, c, p)
		if err != nil {
			return err
		}
		annots, _ := entryArray(x, pd["Annots"])
		var keep types.Array
		var paint strings.Builder
		var res, xobjs types.Dict
		for _, a := range annots {
			w := dictOf(x, a).d
			if w == nil || nameOf(x, w, "Subtype") != "Widget" {
				keep = append(keep, a)
				continue
			}
			// Reference flattening paints saved widget appearances even when
			// Hidden is set. Editable fills retain annotation visibility flags.
			apN := entry(x, dictOf(x, w["AP"]).d, "N")
			var nref types.Object = dictOf(x, w["AP"]).d["N"]
			if nd, ok := apN.(types.Dict); ok {
				as, _ := entry(x, w, "AS").(types.Name)
				nref = nd[as.Value()]
			}
			sd, _ := streamOf(x, nref)
			if sd == nil {
				continue
			}
			bbox, ok1 := numbers(x, sd.Dict["BBox"])
			matrix, ok2 := numbers(x, sd.Dict["Matrix"])
			if !ok2 {
				matrix, ok2 = []float64{1, 0, 0, 1, 0, 0}, true
			}
			rect, ok3 := numbers(x, w["Rect"])
			if !ok1 || !ok3 {
				return errors.New("invalid PDF widget appearance geometry")
			}
			t, err := appearanceTransform(bbox, matrix, rect)
			if err != nil {
				return err
			}
			if res == nil {
				res, xobjs, err = writableResources(x, pd, inh)
				if err != nil {
					return err
				}
			}
			ir, isRef := nref.(types.IndirectRef)
			if !isRef {
				r, err := x.IndRefForNewObject(*sd)
				if err != nil {
					return err
				}
				ir = *r
			}
			if sd.Dict["Subtype"] == nil {
				sd.Dict["Type"] = types.Name("XObject")
				sd.Dict["Subtype"] = types.Name("Form")
			}
			i := 0
			for xobjs[fmt.Sprintf("Fm_widget_%d", i)] != nil {
				i++
			}
			name := fmt.Sprintf("Fm_widget_%d", i)
			xobjs[name] = ir
			fmt.Fprintf(&paint, "q\n%.12f %.12f %.12f %.12f %.12f %.12f cm\n/%s Do\nQ\n", t[0], t[1], t[2], t[3], t[4], t[5], name)
		}
		if paint.Len() > 0 {
			if err := wrapContents(x, pd, []byte(paint.String())); err != nil {
				return err
			}
		}
		if len(annots) > 0 {
			if len(keep) == 0 {
				delete(pd, "Annots")
			} else {
				pd["Annots"] = keep
			}
		}
	}
	cat, err := x.Catalog()
	if err != nil {
		return err
	}
	delete(cat, "AcroForm")
	x.Form = nil
	return nil
}

func writableResources(x *model.XRefTable, pd types.Dict, inh *model.InheritedPageAttrs) (types.Dict, types.Dict, error) {
	res := dictOf(x, pd["Resources"]).d
	if res == nil {
		res = types.Dict{}
		if inh != nil && inh.Resources != nil {
			res = inh.Resources.Clone().(types.Dict)
		}
		pd["Resources"] = res
	}
	xo := dictOf(x, res["XObject"]).d
	if xo == nil {
		xo = types.Dict{}
		res["XObject"] = xo
	}
	return res, xo, nil
}

// wrapContents isolates the existing page stream in q/Q so widget painting
// starts from the initial graphics state, then appends the paint stream.
func wrapContents(x *model.XRefTable, pd types.Dict, paint []byte) error {
	var parts types.Array
	switch v := pd["Contents"].(type) {
	case nil:
	case types.Array:
		parts = append(parts, v...)
	default:
		o, _ := deref(x, v)
		if a, ok := o.(types.Array); ok {
			parts = append(parts, a...)
		} else {
			parts = append(parts, v)
		}
	}
	open, err := addStream(x, []byte("q\n"))
	if err != nil {
		return err
	}
	closing, err := addStream(x, append([]byte("\nQ\n"), paint...))
	if err != nil {
		return err
	}
	all := types.Array{open}
	all = append(all, parts...)
	all = append(all, closing)
	pd["Contents"] = all
	return nil
}
