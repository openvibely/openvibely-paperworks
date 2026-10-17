package pdf

import (
	"context"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Keep source insertion order separately from pdfcpu's map-valued dictionaries.
// This bounded reader uses the validated xref offsets, never scans stream bytes
// for apparent object headers. Compressed objects retain the existing order
// fallback until their source token order is available.
type previewOrder map[string][]string

func dictID(d types.Dict) string { return fmt.Sprintf("%p", d) }
func (o previewOrder) keys(d types.Dict) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, k := range o[dictID(d)] {
		if _, ok := d[k]; ok && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	for _, k := range previewKeys(d) {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}
func previewSpace(s *string) {
	for {
		*s = strings.TrimLeft(*s, "\x00\t\n\f\r ")
		if !strings.HasPrefix(*s, "%") {
			return
		}
		i := strings.IndexAny(*s, "\r\n")
		if i < 0 {
			*s = ""
			return
		}
		*s = (*s)[i:]
	}
}
func (o previewOrder) read(ctx context.Context, s *string, v types.Object, depth int) error {
	if depth > 256 {
		return fmt.Errorf("preview order depth exceeded")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	previewSpace(s)
	if sd, ok := v.(types.StreamDict); ok {
		v = sd.Dict
	}
	if strings.HasPrefix(*s, "<<") {
		d, ok := v.(types.Dict)
		if !ok {
			return fmt.Errorf("preview order dictionary mismatch")
		}
		*s = (*s)[2:]
		keys := []string{}
		for {
			previewSpace(s)
			if strings.HasPrefix(*s, ">>") {
				*s = (*s)[2:]
				break
			}
			key, err := model.ParseObject(ctx, s, 0)
			if err != nil {
				return err
			}
			name, ok := key.(types.Name)
			if !ok {
				return fmt.Errorf("preview dictionary key")
			}
			k := string(name)
			keys = append(keys, k)
			if err = o.read(ctx, s, d[k], depth+1); err != nil {
				return err
			}
		}
		o[dictID(d)] = keys
		return nil
	}
	if strings.HasPrefix(*s, "[") {
		a, ok := v.(types.Array)
		if !ok {
			return fmt.Errorf("preview order array mismatch")
		}
		*s = (*s)[1:]
		for _, x := range a {
			if err := o.read(ctx, s, x, depth+1); err != nil {
				return err
			}
		}
		previewSpace(s)
		if !strings.HasPrefix(*s, "]") {
			return fmt.Errorf("preview array end")
		}
		*s = (*s)[1:]
		return nil
	}
	_, err := model.ParseObject(ctx, s, 0)
	return err
}
func sourcePreviewOrder(ctx context.Context, c *model.Context, data []byte) previewOrder {
	o := previewOrder{}
	source := string(data)
	for n, e := range c.Table {
		if e.Free || e.Compressed || e.Offset == nil || *e.Offset < 0 || *e.Offset >= int64(len(data)) {
			continue
		}
		gen := 0
		if e.Generation != nil {
			gen = *e.Generation
		}
		v, err := c.Dereference(*types.NewIndirectRef(n, gen))
		if err != nil {
			continue
		}
		s := source[*e.Offset:]
		if _, _, err = model.ParseObjectAttributes(&s); err != nil {
			continue
		}
		_ = o.read(ctx, &s, v, 0)
	}
	return o
}
