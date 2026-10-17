/*
Portions derived from pypdf 6.20.0:
Copyright (c) 2006, Mathieu Fenniak
Copyright (c) 2007, Ashish Kulkarni <kulkarni.ashish@gmail.com>

All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

* Redistributions of source code must retain the above copyright notice,
this list of conditions and the following disclaimer.
* Redistributions in binary form must reproduce the above copyright notice,
this list of conditions and the following disclaimer in the documentation
and/or other materials provided with the distribution.
* The name of the author may not be used to endorse or promote products
derived from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE
LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.
*/
package pdf

// Bounded production port of pypdf 6.20.0 TextExtraction and crlf_space_check.
// Reads original stream operands and font widths, not PDFium's completed string.
// Unsupported fonts/operators reject the whole page. Not a general pypdf port.
import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type compatibleTextFont struct {
	codes  map[int]string
	widths map[int]float64
	space  float64
}
type compatibleTextOperation struct {
	op   string
	args []types.Object
}

func compatibleTextNumber(o types.Object) (float64, bool) {
	switch v := o.(type) {
	case types.Integer:
		return float64(v), true
	case types.Float:
		return float64(v), !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
	}
	return 0, false
}
func compatibleTextOperations(ctx context.Context, input string) ([]compatibleTextOperation, error) {
	var ops []compatibleTextOperation
	var args []types.Object
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input = strings.TrimLeft(input, "\x00\t\n\f\r ")
		if input == "" {
			break
		}
		if input[0] == '%' {
			n := strings.IndexAny(input, "\r\n")
			if n < 0 {
				input = ""
				break
			}
			input = input[n:]
			continue
		}
		if strings.ContainsRune("+-.0123456789", rune(input[0])) {
			n := strings.IndexAny(input, "\x00\t\n\f\r /()<>[]%")
			if n < 0 {
				n = len(input)
			}
			token := input[:n]
			if strings.IndexFunc(token, func(r rune) bool { return !strings.ContainsRune("+-.0123456789", r) }) >= 0 {
				return nil, fmt.Errorf("invalid number")
			}
			v, e := strconv.ParseFloat(token, 64)
			if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("invalid number")
			}
			args = append(args, types.Float(v))
			input = input[n:]
			continue
		}
		if strings.ContainsRune("/([<", rune(input[0])) {
			before := len(input)
			r, e := model.ParseObjectWithPolicy(ctx, &input, 0, model.ValidationStrict)
			if e != nil || len(input) >= before {
				return nil, fmt.Errorf("invalid operand: %v", e)
			}
			args = append(args, r.Object)
			continue
		}
		n := strings.IndexAny(input, "\x00\t\n\f\r /()<>[]%")
		if n < 0 {
			n = len(input)
		}
		if n == 0 {
			return nil, fmt.Errorf("unexpected delimiter")
		}
		ops = append(ops, compatibleTextOperation{input[:n], args})
		args = nil
		input = input[n:]
	}
	if len(args) > 0 {
		return nil, fmt.Errorf("trailing operands")
	}
	return ops, nil
}
func compatibleTextPageText(ctx context.Context, c *model.Context, index int) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	var e error
	page, _, attrs, e := c.PageDict(ctx, index+1, false)
	if e != nil {
		return "", e
	}
	raw, e := c.PageContent(page, index+1)
	if e != nil {
		return "", e
	}
	resources, e := c.DereferenceDict(attrs.Resources["Font"])
	if e != nil {
		return "", e
	}
	fonts := map[string]compatibleTextFont{}
	for key, obj := range resources {
		d, e := c.DereferenceDict(obj)
		if e != nil {
			return "", e
		}
		if d["Subtype"] == types.Name("Type1") {
			f, err := compatibleCoreFont(d)
			if err != nil {
				return "", fmt.Errorf("font %s: %w", key, err)
			}
			fonts[key] = f
			continue
		}
		if d["Subtype"] != types.Name("TrueType") || d["Encoding"] != nil {
			return "", fmt.Errorf("unsupported font %s", key)
		}
		stream, e := c.Dereference(d["ToUnicode"])
		if e != nil {
			return "", e
		}
		s, ok := stream.(types.StreamDict)
		if !ok || s.Decode() != nil {
			return "", fmt.Errorf("unsupported CMap %s", key)
		}
		codes, ok := compatibleTextUnicode(s.Content)
		if !ok {
			return "", fmt.Errorf("unsupported CMap %s", key)
		}
		first, ok := compatibleTextNumber(d["FirstChar"])
		if !ok || first != math.Trunc(first) || first < 0 || first > 255 {
			return "", fmt.Errorf("invalid FirstChar")
		}
		widths, e := c.DereferenceArray(d["Widths"])
		if e != nil || len(widths) == 0 || int(first)+len(widths) > 256 {
			return "", fmt.Errorf("invalid Widths")
		}
		desc, e := c.DereferenceDict(d["FontDescriptor"])
		if e != nil {
			return "", e
		}
		_, ok = compatibleTextNumber(desc["Flags"])
		if !ok || desc["MissingWidth"] != nil {
			return "", fmt.Errorf("unsupported font descriptor")
		}
		f := compatibleTextFont{codes: codes, widths: map[int]float64{}}

		spaceCode := 32
		spaceFound := false
		for code, r := range codes {
			if r == " " {
				if spaceFound {
					return "", fmt.Errorf("ambiguous encoded space")
				}
				spaceFound = true
				spaceCode = code
			}
		}
		for i, o := range widths {
			v, e := c.Dereference(o)
			if e != nil {
				return "", e
			}
			w, ok := compatibleTextNumber(v)
			if !ok || w < 0 {
				return "", fmt.Errorf("invalid width")
			}
			f.widths[int(first)+i] = w

		}
		f.space = f.widths[spaceCode]
		if f.space == 0 {
			// PageObject._extract_text overrides a missing encoded space with
			// its default space_width=200, AFTER Font builds fallback widths.
			f.space = 200
		}

		fonts[key] = f
	}
	ops, e := compatibleTextOperations(ctx, string(raw))
	if e != nil {
		return "", e
	}
	// Only image XObjects are text-free under pypdf. Forms remain unsupported.
	images := map[string]bool{}
	if attrs.Resources["XObject"] != nil {
		xobjects, err := c.DereferenceDict(attrs.Resources["XObject"])
		if err != nil {
			return "", err
		}
		for name, object := range xobjects {
			v, err := c.Dereference(object)
			if err != nil {
				return "", err
			}
			if stream, ok := v.(types.StreamDict); ok && stream.Dict["Subtype"] == types.Name("Image") {
				images[name] = true
			}
		}
	}
	return compatibleTextExtract(ctx, ops, fonts, images)
}

func compatibleTextMult(m, n compatibleTextMatrix) compatibleTextMatrix {
	return compatibleTextMatrix{m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3], m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3], m[4]*n[0] + m[5]*n[2] + n[4], m[4]*n[1] + m[5]*n[3] + n[5]}
}
func compatibleTextExtract(ctx context.Context, ops []compatibleTextOperation, fonts map[string]compatibleTextFont, images map[string]bool) (string, error) {
	identity := compatibleTextMatrix{1, 0, 0, 1, 0, 0}
	cm, tm, prevCM, prevTM := identity, identity, identity, identity
	type state struct {
		cm            compatibleTextMatrix
		font          string
		size, leading float64
	}
	var stack []state
	text, output, font := "", "", ""
	size, leading, width, height := 12.0, 0.0, 0.0, 0.0
	halfSpace := 500.0 // pypdf does not save this derived value on its q/Q stack.
	inText := false
	spanAxis := -1
	low, high := 0.0, 0.0
	flush := func() { output += text; text = "" }
	check := func(strWidth float64) {
		prev, m := compatibleTextMult(prevTM, prevCM), compatibleTextMult(tm, cm)
		orientation := 270
		if m[3] > 1e-6 {
			orientation = 0
		} else if m[3] < -1e-6 {
			orientation = 180
		} else if m[1] > 0 {
			orientation = 90
		}
		dx, dy := m[4]-prev[4], m[5]-prev[5]
		axis := 5
		moveH, moveW := dy, dx
		if orientation == 90 || orientation == 270 {
			axis = 4
			moveH, moveW = dx, dy
		}
		distance := math.Abs(moveH)
		if spanAxis == axis {
			distance = 0
			if m[axis] < low || m[axis] > high {
				distance = math.Min(math.Abs(m[axis]-low), math.Abs(m[axis]-high))
			}
		}
		last := ""
		combined := output + text
		if len(combined) > 0 {
			last = combined[len(combined)-1:]
		}
		sy0 := math.Sqrt(prev[2]*prev[2] + prev[3]*prev[3])
		sy := math.Sqrt(m[2]*m[2] + m[3]*m[3])
		sx0 := math.Sqrt(prev[0]*prev[0] + prev[1]*prev[1])
		if distance > 0.8*math.Min(height*sy0, size*sy) {
			if last != "" && last != "\n" {
				output += text + "\n"
				text = ""
			}
			spanAxis, low, high = axis, m[axis], m[axis]
		} else {
			if moveW >= (size*(halfSpace)/1000+strWidth)*sx0 && last != "" && last != " " {
				text += " "
			}
			if last == "" || last == "\n" || spanAxis != axis {
				spanAxis, low, high = axis, m[axis], m[axis]
			} else {
				low = math.Min(low, m[axis])
				high = math.Max(high, m[axis])
			}
		}
		prevCM, prevTM = cm, tm
	}
	show := func(o types.Object, synthetic bool) error {
		f, ok := fonts[font]
		if !ok {
			return fmt.Errorf("unknown font")
		}
		var raw []byte
		var e error
		switch v := o.(type) {
		case types.HexLiteral:
			raw, e = hex.DecodeString(string(v))
		case types.StringLiteral:
			raw, e = types.Unescape(string(v))
		default:
			return fmt.Errorf("invalid text")
		}
		if e != nil {
			return e
		}
		w := 0.0
		for _, code := range raw {
			if synthetic {
				text += " "
				w += f.space
				continue
			}
			r, ok := f.codes[int(code)]
			v, valid := f.widths[int(code)]
			if !ok || !valid {
				return fmt.Errorf("unmapped code %d", code)
			}
			for _, ch := range r {
				if ch >= 0x590 && ch <= 0x8ff || ch >= 0xfb1d && ch <= 0xfdff || ch >= 0xfe70 && ch <= 0xfeff {
					return fmt.Errorf("unsupported bidi")
				}
			}
			text += r
			if r == " " {
				v = f.space
			}
			w += v
		}
		width += w * size
		height = size
		check(0)
		return nil
	}
	for _, o := range ops {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		a := o.args
		fail := func() (string, error) { return "", fmt.Errorf("unsupported/malformed %s", o.op) }
		numbers := func(n int) ([]float64, bool) {
			if len(a) != n {
				return nil, false
			}
			v := make([]float64, n)
			for i, x := range a {
				var ok bool
				v[i], ok = compatibleTextNumber(x)
				if !ok {
					return nil, false
				}
			}
			return v, true
		}
		switch o.op {
		case "Do":
			if inText || len(a) != 1 {
				return fail()
			}
			name, ok := a[0].(types.Name)
			if !ok || !images[string(name)] {
				return fail()
			}
			// PageObject._extract_text flushes and terminates pending text even
			// when _extract_text__xform returns None for an image (no OCR).
			flush()
			if output != "" && !strings.HasSuffix(output, "\n") {
				output += "\n"
			}
		case "q":
			if len(a) != 0 || inText {
				return fail()
			}
			stack = append(stack, state{cm, font, size, leading})
		case "Q":
			if len(a) != 0 || inText || len(stack) == 0 {
				return fail()
			}
			s := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			cm, font, size, leading = s.cm, s.font, s.size, s.leading
		case "cm":
			v, ok := numbers(6)
			if !ok || inText {
				return fail()
			}
			flush()
			cm = compatibleTextMult(compatibleTextMatrix(v), cm)
		case "BT":
			if len(a) != 0 || inText {
				return fail()
			}
			inText = true
			tm = identity
			flush()
		case "ET":
			if len(a) != 0 || !inText {
				return fail()
			}
			inText = false
			flush()
		case "Tf":
			if !inText || len(a) != 2 {
				return fail()
			}
			n, ok := a[0].(types.Name)
			v, valid := compatibleTextNumber(a[1])
			if !ok || !valid || v <= 0 {
				return fail()
			}
			if _, ok := fonts[string(n)]; !ok {
				return fail()
			}
			flush()
			font, size = string(n), v
			halfSpace = fonts[font].space / 2
		case "Tm":
			v, ok := numbers(6)
			if !ok || !inText {
				return fail()
			}
			tm = compatibleTextMatrix(v)
			w := width / 1000
			width = 0
			check(w)
		case "Td", "TD":
			v, ok := numbers(2)
			if !ok || !inText {
				return fail()
			}
			if o.op == "TD" {
				leading = -v[1]
			}
			tm[4] += v[0]*tm[0] + v[1]*tm[2]
			tm[5] += v[0]*tm[1] + v[1]*tm[3]
			w := width / 1000
			width = 0
			check(w)
		case "TL":
			v, ok := numbers(1)
			if !ok || !inText {
				return fail()
			}
			leading = v[0]
		case "T*":
			if len(a) != 0 || !inText {
				return fail()
			}
			tm[4] -= leading * tm[2]
			tm[5] -= leading * tm[3]
			w := width / 1000
			width = 0
			check(w)
		case "Tj":
			if !inText || len(a) != 1 {
				return fail()
			}
			if e := show(a[0], false); e != nil {
				return "", e
			}
		case "TJ":
			if !inText || len(a) != 1 {
				return fail()
			}
			seq, ok := a[0].(types.Array)
			if !ok {
				return fail()
			}
			for _, x := range seq {
				if n, ok := compatibleTextNumber(x); ok {
					if math.Abs(n) >= halfSpace*.95 && text != "" && !strings.HasSuffix(text, " ") {
						if e := show(types.StringLiteral(" "), true); e != nil {
							return "", e
						}
					}
				} else if e := show(x, false); e != nil {
					return "", e
				}
			}
		case "rg", "RG", "w", "re", "m", "l", "c", "J", "j":
			arity := map[string]int{"rg": 3, "RG": 3, "w": 1, "re": 4, "m": 2, "l": 2, "c": 6, "J": 1, "j": 1}[o.op]
			if _, ok := numbers(arity); !ok {
				return fail()
			}
		case "W*", "W", "n", "S", "h", "f*", "f", "B*", "B":
			if len(a) != 0 {
				return fail()
			}
		case "BMC":
			if len(a) != 1 {
				return fail()
			}
			if _, ok := a[0].(types.Name); !ok {
				return fail()
			}
		case "BDC":
			if len(a) != 2 {
				return fail()
			}
			if _, ok := a[0].(types.Name); !ok {
				return fail()
			}
			if _, ok := a[1].(types.Dict); !ok {
				return fail()
			}
		case "EMC":
			if len(a) != 0 {
				return fail()
			}
		default:
			return fail()
		}
	}
	if inText || len(stack) != 0 {
		return "", fmt.Errorf("unbalanced page")
	}
	flush()
	return output, nil
}

// Same bounded bfchar grammar as the metric reader, but retain multi-character
// BMP destinations (e.g. one glyph mapping to "fl") as one encoded character.
func compatibleTextUnicode(raw []byte) (map[int]string, bool) {
	grammar := regexp.MustCompile(strings.ReplaceAll(compatibleTextCMap.String(), "[0-9A-Fa-f]{4}", "(?:[0-9A-Fa-f]{4})+"))
	match := grammar.FindSubmatch(raw)
	if match == nil {
		return nil, false
	}
	pairs := regexp.MustCompile(`<([0-9A-Fa-f]{2})>\s*<((?:[0-9A-Fa-f]{4})+)>`).FindAllSubmatch(match[2], -1)
	count, e := strconv.Atoi(string(match[1]))
	if e != nil || count != len(pairs) {
		return nil, false
	}
	out := map[int]string{}
	for _, p := range pairs {
		code, _ := strconv.ParseInt(string(p[1]), 16, 32)
		if _, ok := out[int(code)]; ok {
			return nil, false
		}
		var text strings.Builder
		for i := 0; i < len(p[2]); i += 4 {
			r, _ := strconv.ParseInt(string(p[2][i:i+4]), 16, 32)
			if r >= 0xd800 && r <= 0xdfff {
				return nil, false
			}
			text.WriteRune(rune(r))
		}
		out[int(code)] = text.String()
	}
	return out, true
}

// PypdfPageText reconstructs text using the verified pypdf 6.20 subset.
// Only simple TrueType fonts with one-byte bfchar CMaps and the supported text
// state are accepted. Unsupported pages fail the entire document with no partial
// strings and no PDFium fallback. This is not general pypdf compatibility.
func PypdfPageText(ctx context.Context, data []byte) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := Load(ctx, data)
	if err != nil {
		return nil, err
	}
	out := make([]string, c.PageCount)
	for i := range out {
		out[i], err = compatibleTextPageText(ctx, c, i)
		if err != nil {
			return nil, fmt.Errorf("pypdf text page %d: %w", i+1, err)
		}
	}
	return out, nil
}

type compatibleTextMatrix [6]float64

var compatibleTextCMap = regexp.MustCompile(`(?s)^\s*/CIDInit/ProcSet\s+findresource\s+begin\s+12\s+dict\s+begin\s+begincmap\s+/CIDSystemInfo\s*<<\s*/Registry\s+\(Adobe\)\s*/Ordering\s+\(UCS\)\s*/Supplement\s+0\s*>>\s*def\s*/CMapName/Adobe-Identity-UCS\s+def\s*/CMapType\s+2\s+def\s+1\s+begincodespacerange\s*<00>\s*<FF>\s*endcodespacerange\s+(\d+)\s+beginbfchar\s*((?:<[0-9A-Fa-f]{2}>\s*<[0-9A-Fa-f]{4}>\s*)+)endbfchar\s+endcmap\s+CMapName\s+currentdict\s+/CMap\s+defineresource\s+pop\s+end\s+end\s*$`)

// Keep custom widths, encodings, descriptors and CMaps out of this bounded path.
func compatibleCoreFont(d types.Dict) (compatibleTextFont, error) {
	fail := func() (compatibleTextFont, error) { return compatibleTextFont{}, fmt.Errorf("unsupported Type1 font") }
	if d["Encoding"] != types.Name("WinAnsiEncoding") {
		return fail()
	}
	for key := range d {
		switch key {
		case "Type", "Subtype", "BaseFont", "Encoding", "Name":
		default:
			return fail()
		}
	}
	name, ok := d["BaseFont"].(types.Name)
	if !ok {
		return fail()
	}
	widths, ok := compatibleCoreWidths[string(name)]
	if !ok {
		return fail()
	}
	f := compatibleTextFont{codes: map[int]string{}, widths: map[int]float64{}, space: widths[0]}
	for i, w := range widths {
		c := i + 32
		ch := string(rune(c))
		if i == 95 {
			c = 151
			ch = "—"
		}
		f.codes[c] = ch
		f.widths[c] = w
	}
	return f, nil
}
