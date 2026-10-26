package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func init() { api.DisableConfigDir() }

func config() *model.Configuration {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	return conf
}

// Load parses and validates a PDF held in memory.
func Load(ctx context.Context, data []byte) (*model.Context, error) {
	return load(ctx, data, nil)
}

func load(ctx context.Context, data []byte, onPrepare func()) (*model.Context, error) {
	if onPrepare != nil {
		onPrepare()
	}
	c, err := api.ReadAndValidate(ctx, bytes.NewReader(data), config())
	if err != nil {
		return nil, fmt.Errorf("read PDF: %w", err)
	}
	if err := c.EnsurePageCount(); err != nil {
		return nil, err
	}
	return c, nil
}

func LoadFile(ctx context.Context, path string) (*model.Context, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	c, err := Load(ctx, data)
	return c, data, err
}

func Serialize(ctx context.Context, c *model.Context) ([]byte, error) {
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, c, &buf); err != nil {
		return nil, fmt.Errorf("write PDF: %w", err)
	}
	return buf.Bytes(), nil
}

type ref struct {
	d   types.Dict
	num int // 0 when the object is direct
}

func deref(x *model.XRefTable, o types.Object) (types.Object, int) {
	seen := 0
	num := 0
	for {
		ir, ok := o.(types.IndirectRef)
		if !ok {
			return o, num
		}
		num = ir.ObjectNumber.Value()
		v, err := x.Dereference(ir)
		if err != nil || seen > 32 {
			return nil, num
		}
		o = v
		seen++
	}
}

func dictOf(x *model.XRefTable, o types.Object) ref {
	v, n := deref(x, o)
	switch d := v.(type) {
	case types.Dict:
		return ref{d, n}
	case types.StreamDict:
		return ref{d.Dict, n}
	}
	return ref{nil, n}
}

func entry(x *model.XRefTable, d types.Dict, key string) types.Object {
	if d == nil {
		return nil
	}
	v, ok := d[key]
	if !ok {
		return nil
	}
	o, _ := deref(x, v)
	return o
}

func nameOf(x *model.XRefTable, d types.Dict, key string) string {
	if n, ok := entry(x, d, key).(types.Name); ok {
		return n.Value()
	}
	return ""
}

func intOf(x *model.XRefTable, d types.Dict, key string) int {
	switch v := entry(x, d, key).(type) {
	case types.Integer:
		return v.Value()
	case types.Float:
		return int(v.Value())
	}
	return 0
}

func textOf(o types.Object) (string, bool) {
	switch v := o.(type) {
	case types.StringLiteral:
		s, err := types.StringLiteralToString(v)
		return s, err == nil
	case types.HexLiteral:
		s, err := types.HexLiteralToString(v)
		return s, err == nil
	}
	return "", false
}

// display renders a PDF value the way callers pass it back: names keep a
// leading slash so button states are distinguishable from text.
func display(x *model.XRefTable, o types.Object) *string {
	o, _ = deref(x, o)
	var s string
	switch v := o.(type) {
	case nil:
		return nil
	case types.Name:
		s = "/" + v.Value()
	case types.Integer:
		s = strconv.Itoa(v.Value())
	case types.Float:
		s = strconv.FormatFloat(v.Value(), 'f', -1, 64)
	case types.Boolean:
		s = strconv.FormatBool(v.Value())
	case types.Array:
		parts := make([]string, 0, len(v))
		for _, e := range v {
			if p := display(x, e); p != nil {
				parts = append(parts, *p)
			}
		}
		s = "[" + strings.Join(parts, ", ") + "]"
	default:
		t, ok := textOf(o)
		if !ok {
			s = o.String()
		} else {
			s = t
		}
	}
	return &s
}

func numbers(x *model.XRefTable, o types.Object) ([]float64, bool) {
	a, ok := entryArray(x, o)
	if !ok {
		return nil, false
	}
	out := make([]float64, 0, len(a))
	for _, e := range a {
		e, _ = deref(x, e)
		switch v := e.(type) {
		case types.Integer:
			out = append(out, float64(v.Value()))
		case types.Float:
			out = append(out, v.Value())
		default:
			return nil, false
		}
	}
	return out, true
}

func entryArray(x *model.XRefTable, o types.Object) (types.Array, bool) {
	o, _ = deref(x, o)
	a, ok := o.(types.Array)
	return a, ok
}

func streamOf(x *model.XRefTable, o types.Object) (*types.StreamDict, int) {
	v, n := deref(x, o)
	if sd, ok := v.(types.StreamDict); ok {
		return &sd, n
	}
	return nil, n
}

func addStream(x *model.XRefTable, content []byte) (types.IndirectRef, error) {
	sd, err := x.NewStreamDictForBuf(content)
	if err != nil {
		return types.IndirectRef{}, err
	}
	if err := sd.Encode(); err != nil {
		return types.IndirectRef{}, err
	}
	ir, err := x.IndRefForNewObject(*sd)
	if err != nil {
		return types.IndirectRef{}, err
	}
	return *ir, nil
}

func pageDict(ctx context.Context, c *model.Context, n int) (types.Dict, *model.InheritedPageAttrs, error) {
	d, _, inh, err := c.PageDict(ctx, n, false)
	if err != nil {
		return nil, nil, err
	}
	if d == nil {
		return nil, nil, fmt.Errorf("page %d missing", n)
	}
	return d, inh, nil
}
