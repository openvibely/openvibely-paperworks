// Selected-page cloning and FloatObject formatting follow pypdf 6.20.0.
// The pinned source hashes and upstream BSD license are retained in
// testdata/preview-writer/reference.json and PYPDF_LICENSE.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// SelectPreviewPages copies the page graph into a fresh document, as the pinned
// preview helper's PdfWriter/add_page does. It does not optimize resources or
// copy the source catalog/Info. General user-facing page operations retain their
// existing writer. Uncompressed source dictionaries retain their insertion order.
func SelectPreviewPages(ctx context.Context, data []byte, pages []int) ([]byte, error) {
	c, err := Load(ctx, data)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages selected")
	}
	// The generic collector retains its encryption handling.
	if c.Encrypt != nil {
		return SelectPages(ctx, data, pages)
	}
	order := sourcePreviewOrder(ctx, c, data)
	inherited := map[string]types.IndirectRef{}
	ir := func(n int) types.IndirectRef { return *types.NewIndirectRef(n, 0) }
	objects := []types.Object{types.Dict{"Producer": types.StringLiteral("pypdf")}, nil, types.Dict{"Type": types.Name("Catalog"), "Pages": ir(2)}}
	order[dictID(objects[2].(types.Dict))] = []string{"Type", "Pages"}
	translated := map[types.IndirectRef]types.IndirectRef{}
	var clone func(types.Object, int) (types.Object, error)
	clone = func(o types.Object, depth int) (types.Object, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if depth > 256 || len(objects) > 100000 {
			return nil, fmt.Errorf("preview graph exceeds bounds")
		}
		switch v := o.(type) {
		case types.IndirectRef:
			if dst, ok := translated[v]; ok {
				return dst, nil
			}
			dst := ir(len(objects) + 1)
			translated[v] = dst
			objects = append(objects, nil)
			src, e := c.Dereference(v)
			if e != nil {
				return nil, e
			}
			out, e := clone(src, depth+1)
			if e != nil {
				return nil, e
			}
			objects[dst.ObjectNumber.Value()-1] = out
			return dst, nil
		case types.Dict:
			var orphan *types.IndirectRef
			if src, ok := inherited[dictID(v)]; ok {
				if _, done := translated[src]; !done {
					dst := ir(len(objects) + 1)
					translated[src] = dst
					objects = append(objects, nil)
					orphan = &dst
				}
			}
			out := types.Dict{}
			keys := order.keys(v)
			order[dictID(out)] = keys
			for _, k := range keys {
				x, e := clone(v[k], depth+1)
				if e != nil {
					return nil, e
				}
				out[k] = x
			}
			if orphan != nil {
				objects[orphan.ObjectNumber.Value()-1] = out
			}
			return out, nil
		case types.Array:
			out := make(types.Array, len(v))
			for i, x := range v {
				y, e := clone(x, depth+1)
				if e != nil {
					return nil, e
				}
				out[i] = y
			}
			return out, nil
		case types.StreamDict:
			// Length is always rewritten directly; copying an indirect length would
			// create an unreachable extra object absent from pypdf's encoded stream.
			d := types.Dict{}
			for k, x := range v.Dict {
				if k != "Length" {
					d[k] = x
				}
			}
			order[dictID(d)] = order.keys(v.Dict)
			q, e := clone(d, depth+1)
			if e != nil {
				return nil, e
			}
			v.Dict = q.(types.Dict)
			v.Dict["Length"] = types.Integer(len(v.Raw))
			return v, nil
		default:
			return o, nil
		}
	}
	kids := types.Array{}
	for _, n := range pages {
		d, ref, _, e := c.PageDict(ctx, n, false)
		if e != nil {
			return nil, e
		}
		// Annotation link repair requires a separate contract. Keep its established
		// implementation until the dedicated writer supports it.
		if _, ok := d["Annots"]; ok {
			return SelectPages(ctx, data, pages)
		}
		page := types.Dict{}
		for k, v := range d {
			if k != "Parent" && k != "StructParents" {
				page[k] = v
			}
		}
		order[dictID(page)] = order.keys(d)
		parent := d["Parent"]
		seen := map[types.IndirectRef]bool{}
		for parent != nil {
			if p, ok := parent.(types.IndirectRef); ok {
				if seen[p] {
					return nil, fmt.Errorf("cyclic page parents")
				}
				seen[p] = true
			}
			if len(seen) > 256 {
				return nil, fmt.Errorf("page parent depth exceeds bounds")
			}
			p, e := c.DereferenceDict(parent)
			if e != nil {
				return nil, e
			}
			if p == nil {
				break
			}
			for _, k := range []string{"Resources", "MediaBox", "CropBox", "Rotate"} {
				if _, ok := page[k]; !ok {
					if v, found := p[k]; found {
						resolved, err := c.Dereference(v)
						if err != nil {
							return nil, err
						}
						page[k] = resolved
						order[dictID(page)] = append(order[dictID(page)], k)
						if ref, ok := v.(types.IndirectRef); ok {
							if d, ok := resolved.(types.Dict); ok {
								inherited[dictID(d)] = ref
							}
						}
					}
				}
			}
			parent = p["Parent"]
		}
		dst := ir(len(objects) + 1)
		translated[*ref] = dst
		objects = append(objects, nil)
		q, e := clone(page, 0)
		if e != nil {
			return nil, e
		}
		out := q.(types.Dict)
		out["Parent"] = ir(2)
		order[dictID(out)] = append(order[dictID(out)], "Parent")
		objects[dst.ObjectNumber.Value()-1] = out
		kids = append(kids, dst)
	}
	objects[1] = types.Dict{"Type": types.Name("Pages"), "Count": types.Integer(len(kids)), "Kids": kids}
	order[dictID(objects[1].(types.Dict))] = []string{"Type", "Count", "Kids"}
	var b bytes.Buffer
	header := "%PDF-1.3"
	if len(data) >= 8 && bytes.HasPrefix(data, []byte("%PDF-1.")) {
		if data[7] > '3' {
			header = string(data[:8])
		}
	}
	fmt.Fprintf(&b, "%s\n%%\xe2\xe3\xcf\xd3\n", header)
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", i+1)
		if e := writeOrderedPreviewObject(&b, o, order); e != nil {
			return nil, e
		}
		b.WriteString("\nendobj\n")
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, n := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", n)
	}
	fmt.Fprintf(&b, "trailer\n<<\n/Size %d\n/Root 3 0 R\n/Info 1 0 R\n>>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, x)
	return b.Bytes(), nil
}
func previewKeys(d types.Dict) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func writePreviewObject(b *bytes.Buffer, o types.Object) error {
	return writeOrderedPreviewObject(b, o, nil)
}
func writeOrderedPreviewObject(b *bytes.Buffer, o types.Object, order previewOrder) error {
	switch v := o.(type) {
	case nil:
		b.WriteString("null")
	case types.Dict:
		b.WriteString("<<\n")
		for _, k := range order.keys(v) {
			fmt.Fprintf(b, "/%s ", types.EncodeName(k))
			if e := writeOrderedPreviewObject(b, v[k], order); e != nil {
				return e
			}
			b.WriteByte('\n')
		}
		b.WriteString(">>")
	case types.Array:
		b.WriteString("[ ")
		for _, x := range v {
			if e := writeOrderedPreviewObject(b, x, order); e != nil {
				return e
			}
			b.WriteByte(' ')
		}
		b.WriteByte(']')
	case types.StringLiteral:
		raw, err := types.Unescape(string(v))
		if err != nil {
			return err
		}
		writePreviewString(b, raw)
	case types.HexLiteral:
		raw, err := v.Bytes()
		if err != nil {
			return err
		}
		writePreviewString(b, raw)
	case types.Float:
		// pypdf FloatObject.myrepr uses eight significant decimal places
		// with truncation of log10 toward zero, then strips trailing zeroes.
		if v == 0 {
			b.WriteString("0.0")
		} else {
			n := max(1, 8-int(math.Log10(math.Abs(float64(v)))))
			b.WriteString(strings.TrimRight(strings.TrimRight(strconv.FormatFloat(float64(v), 'f', n, 64), "0"), "."))
		}
	case types.StreamDict:
		if e := writeOrderedPreviewObject(b, v.Dict, order); e != nil {
			return e
		}
		b.WriteString("\nstream\n")
		b.Write(v.Raw)
		b.WriteString("\nendstream")
	default:
		b.WriteString(o.PDFString())
	}
	return nil
}
