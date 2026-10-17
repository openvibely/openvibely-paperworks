package pdf

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Recover only the malformed direct empty /AP /N stream emitted by pypdf when
// assigning DecodedStreamObject() directly. This is report-only: never rewrite
// input bytes or feed a recovered graph into filling/flattening. Offsets and
// object identities come from the parsed cross-reference table, not a text scan.
func recoverEmptyWidgetAppearances(ctx context.Context, c *model.Context, data []byte) {
	if c.Encrypt != nil {
		return
	}
	offsets := []int64{int64(len(data))}
	for _, entry := range c.Table {
		if entry.Offset != nil && !entry.Free && !entry.Compressed && *entry.Offset >= 0 && *entry.Offset < int64(len(data)) {
			offsets = append(offsets, *entry.Offset)
		}
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	for number, e := range c.Table {
		if ctx.Err() != nil {
			return
		}
		if e.Free || e.Compressed || e.Object != nil || e.Offset == nil || e.Generation == nil {
			continue
		}
		start := *e.Offset
		if start < 0 || start >= int64(len(data)) {
			continue
		}
		end := offsets[sort.Search(len(offsets), func(i int) bool { return offsets[i] > start })]
		// Bound recovery work; oversized/ambiguous objects keep the warning path.
		if end-start > 1<<20 {
			continue
		}
		text := string(data[start:end])
		n, g, err := model.ParseObjectAttributes(&text)
		if err != nil || *n != number || *g != *e.Generation {
			continue
		}
		parser := emptyAppearanceParser{ctx: ctx, text: text}
		obj, err := parser.read(0, "")
		parser.space()
		if err != nil || !parser.recovered || !strings.HasPrefix(parser.text, "endobj") {
			continue
		}
		// The bound can include an xref/trailer; endobj itself must be delimited.
		if len(parser.text) > 6 && !strings.ContainsRune("\x00\t\n\f\r %", rune(parser.text[6])) {
			continue
		}
		d, ok := obj.(types.Dict)
		if !ok || d.NameEntry("Subtype") == nil || *d.NameEntry("Subtype") != "Widget" {
			continue
		}
		e.Object = d
	}
}

type emptyAppearanceParser struct {
	ctx       context.Context
	text      string
	recovered bool
}

func (p *emptyAppearanceParser) space() {
	for {
		p.text = strings.TrimLeft(p.text, "\x00\t\n\f\r ")
		if !strings.HasPrefix(p.text, "%") {
			return
		}
		i := strings.IndexAny(p.text, "\r\n")
		if i < 0 {
			p.text = ""
			return
		}
		p.text = p.text[i:]
	}
}
func (p *emptyAppearanceParser) read(depth int, path string) (types.Object, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	if depth > 32 {
		return nil, errors.New("recovery nesting limit")
	}
	p.space()
	if strings.HasPrefix(p.text, "<<") {
		p.text = p.text[2:]
		d := types.Dict{}
		for {
			p.space()
			if strings.HasPrefix(p.text, ">>") {
				p.text = p.text[2:]
				break
			}
			if !strings.HasPrefix(p.text, "/") {
				return nil, errors.New("dictionary key required")
			}
			key, err := model.ParseObject(p.ctx, &p.text, 0)
			if err != nil {
				return nil, err
			}
			name, ok := key.(types.Name)
			if !ok {
				return nil, errors.New("dictionary name required")
			}
			if _, exists := d[string(name)]; exists {
				return nil, errors.New("duplicate dictionary key")
			}
			value, err := p.read(depth+1, path+"/"+string(name))
			if err != nil {
				return nil, err
			}
			d[string(name)] = value
		}
		p.space()
		if strings.HasPrefix(p.text, "stream") {
			length, ok := d["Length"].(types.Integer)
			if path != "/AP/N" || !ok || length != 0 || len(d) != 1 {
				return nil, errors.New("unsupported direct stream")
			}
			p.text = p.text[6:]
			if strings.HasPrefix(p.text, "\r\n") {
				p.text = p.text[2:]
			} else if strings.HasPrefix(p.text, "\n") {
				p.text = p.text[1:]
			} else {
				return nil, errors.New("stream EOL required")
			}
			// Zero data bytes followed by the optional separator EOL.
			if strings.HasPrefix(p.text, "\r\n") {
				p.text = p.text[2:]
			} else if strings.HasPrefix(p.text, "\n") {
				p.text = p.text[1:]
			}
			if !strings.HasPrefix(p.text, "endstream") {
				return nil, errors.New("nonempty or unterminated stream")
			}
			p.text = p.text[9:]
			if len(p.text) > 0 && !strings.ContainsRune("\x00\t\n\f\r >", rune(p.text[0])) {
				return nil, errors.New("stream terminator boundary")
			}
			p.recovered = true
			return types.StreamDict{Dict: d, Content: []byte{}, Raw: []byte{}}, nil
		}
		return d, nil
	}
	// Standard scalars and arrays retain pdfcpu's parser; nested direct streams
	// in arrays are intentionally outside this narrow recovery contract.
	return model.ParseObject(p.ctx, &p.text, 0)
}
