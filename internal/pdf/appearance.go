package pdf

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	pdffont "github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// restoreWidgetDecoration replaces only the decoration emitted by our appearance
// generator. Keep the widget's full color precision and solid border; pdfcpu's
// text and combo generators round colors and can omit the widget border.
// The reference redraws dashed widgets with solid borders while retaining /BS.
// Other border styles retain the generator's behavior. The reference keeps
// right-angle text widget rotation in MK without rotating the saved appearance.
func restoreWidgetDecoration(x *model.XRefTable, w types.Dict) error {
	mk := dictOf(x, w["MK"]).d
	if mk == nil || !supportedSavedTextRotation(x, w) {
		return nil
	}
	bs := dictOf(x, w["BS"]).d
	if style := nameOf(x, bs, "S"); style != "" && style != "S" && style != "D" {
		return nil
	}
	if w["Border"] != nil && bs == nil {
		return nil
	}
	width := 1.0
	if bs["W"] != nil {
		a, ok := numbers(x, types.Array{bs["W"]})
		if !ok {
			return nil
		}
		width = a[0]
	}
	if math.IsNaN(width) || math.IsInf(width, 0) || width < 0 {
		return nil
	}
	ap := dictOf(x, w["AP"]).d
	sd, _ := streamOf(x, ap["N"])
	if sd == nil {
		return nil
	}
	box, ok := numbers(x, sd.Dict["BBox"])
	if !ok || len(box) != 4 || box[0] != 0 || box[1] != 0 {
		return nil
	}
	if err := sd.Decode(); err != nil {
		return err
	}
	marker := bytes.Index(sd.Content, []byte("/Tx BMC"))
	if marker < 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("q\n")
	if color := widgetColor(x, mk["BG"], false); color != "" {
		fmt.Fprintf(&b, "%s\n0 0 %g %g re f\n", color, box[2], box[3])
	}
	if color := widgetColor(x, mk["BC"], true); color != "" && width > 0 {
		fmt.Fprintf(&b, "%s\n%g w\n%g %g %g %g re s\n", color, width, width, width, box[2]-2*width, box[3]-2*width)
	}
	b.WriteString("Q\n")
	b.Write(sd.Content[marker:])
	sd.Content = []byte(b.String())
	if err := sd.Encode(); err != nil {
		return err
	}
	ref, err := x.IndRefForNewObject(*sd)
	if err != nil {
		return err
	}
	ap["N"] = *ref
	return nil
}

// supportedSavedTextRotation limits the observed rotation compatibility to
// single-line text widgets. Preserve MK/R; this is saved-stream equivalence only.
func supportedSavedTextRotation(x *model.XRefTable, w types.Dict) bool {
	rotation := intOf(x, dictOf(x, w["MK"]).d, "R")
	if rotation == 0 {
		return true
	}
	return nameOf(x, w, "FT") == "Tx" && intOf(x, w, "Ff")&((1<<12)|(1<<24)) == 0 &&
		(rotation == 90 || rotation == 180 || rotation == 270)
}

func widgetColor(x *model.XRefTable, obj types.Object, stroke bool) string {
	values, ok := numbers(x, obj)
	if !ok {
		return ""
	}
	op := map[int]string{1: "g", 3: "rg", 4: "k"}[len(values)]
	if op == "" {
		return ""
	}
	var b strings.Builder
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return ""
		}
		fmt.Fprintf(&b, "%g ", v)
	}
	if stroke {
		op = strings.ToUpper(op)
	}
	b.WriteString(op)
	return b.String()
}

// These patterns match only the prefix produced by pdfcpu, before the text
// literal. Never parse or rewrite user text as PDF operators.
var appearancePrefix = regexp.MustCompile(`(?s)^(/Tx BMC q )[-0-9.]+ [-0-9.]+ [-0-9.]+ [-0-9.]+ re W n (BT /([^ ]+) ([0-9.]+) Tf .*? )[-0-9.]+ [-0-9.]+ Td \(`)
var appearanceStrokeColor = regexp.MustCompile(`[-0-9.]+ [-0-9.]+ [-0-9.]+ RG `)
var listAppearancePrefix = regexp.MustCompile(`(?s)^(/Tx BMC q ).*?(BT /([^ ]+) ([0-9.]+) Tf .*? )[-0-9.]+ [-0-9.]+ Td \(`)
var savedRGBDefault = regexp.MustCompile(`^\s*/[^ ]+ [0-9.]+ Tf\s+([0-9.]+) ([0-9.]+) ([0-9.]+) rg\s*$`)
var generatedRGBColor = regexp.MustCompile(`[-0-9.]+ [-0-9.]+ [-0-9.]+ (?:rg|RG) `)

// Preserve the full precision of a simple RGB default appearance. Only rewrite
// the generated operator prefix; field text must never be parsed as operators.
func preserveRGBDefault(prefix, da string) string {
	match := savedRGBDefault.FindStringSubmatch(da)
	if match == nil {
		return prefix
	}
	for _, component := range match[1:] {
		v, err := strconv.ParseFloat(component, 64)
		if err != nil || v < 0 || v > 1 {
			return prefix
		}
	}
	return generatedRGBColor.ReplaceAllStringFunc(prefix, func(op string) string {
		return strings.Join(match[1:], " ") + " " + op[len(op)-3:]
	})
}

var fixedFontSize = regexp.MustCompile(`/[^ ]+ ([0-9.]+) Tf`)

// alignSingleLineAppearance uses font ascent rather than the font bounding box
// to center fixed-size Helvetica-family, Courier-family or supported Times text, and lays out explicit
// lines for fixed-size supported Helvetica-family, Courier-family and supported Times multiline fields
// with standard quadding. Fixed-size Helvetica combs center glyphs in cells.
// Single-line ASCII Helvetica also supports automatic sizing.
// Other layouts keep their existing generator until their metrics and wrapping are independently checked.
func alignSingleLineAppearance(x *model.XRefTable, w types.Dict, f *field, value string, da *string) error {
	listbox := f.FT == "Ch" && f.Ff&ffCombo == 0
	selected := value
	if listbox {
		// The reference rebuilds the saved appearance from all options even
		// when TI is present. Preserve TI for the viewer without offsetting
		// this appearance or falling back to the generator's older layout.
		opts, ok := entryArray(x, f.D["Opt"])
		if !ok {
			return nil
		}
		for _, opt := range opts {
			if _, paired := entryArray(x, opt); paired {
				return nil
			}
		}
		_, labels := choiceOptions(x, f.D)
		value = strings.Join(labels, "\n")
	}
	comb := f.FT == "Tx" && f.Ff&(1<<24) != 0
	multiline := f.FT == "Tx" && f.Ff&(1<<12) != 0
	if multiline {
		// The reference treats vertical tab and form feed as line breaks.
		// Change only layout text; the stored field value remains untouched.
		value = strings.NewReplacer("\v", "\n", "\f", "\n").Replace(value)
	}
	if (f.FT != "Tx" && !(f.FT == "Ch" && (f.Ff&ffCombo != 0 || listbox))) || (comb && multiline) || (!multiline && !listbox && strings.ContainsAny(value, "\r\n")) {
		return nil
	}
	alignment := intOf(x, f.D, "Q")
	if alignment < 0 || alignment > 2 {
		return nil
	}
	if da == nil {
		return nil
	}
	size := fixedFontSize.FindStringSubmatch(*da)
	if len(size) == 0 {
		return nil
	}
	points, _ := strconv.ParseFloat(size[1], 64)
	autoSize := points == 0
	if points < 0 || (autoSize && (f.FT != "Tx" || multiline || comb)) {
		return nil
	}
	if !supportedSavedTextRotation(x, w) {
		return nil
	}
	bs := dictOf(x, w["BS"]).d
	if style := nameOf(x, bs, "S"); style != "" && style != "S" && style != "D" {
		return nil
	}
	if w["Border"] != nil && bs == nil {
		return nil
	}
	// No border dictionary means no vertical clipping inset.
	margin := 0.0
	if bs != nil {
		margin = 1
	}
	if bs["W"] != nil {
		a, ok := numbers(x, types.Array{bs["W"]})
		if !ok {
			return nil
		}
		margin = a[0]
	}
	if margin < 0 || math.IsNaN(margin) || math.IsInf(margin, 0) {
		return nil
	}
	ap := dictOf(x, w["AP"]).d
	sd, _ := streamOf(x, ap["N"])
	if sd == nil {
		return nil
	}
	box, ok := numbers(x, sd.Dict["BBox"])
	if !ok || len(box) != 4 || box[0] != 0 || box[1] != 0 {
		return nil
	}
	if err := sd.Decode(); err != nil {
		return err
	}
	start := bytes.Index(sd.Content, []byte("/Tx BMC"))
	if start < 0 {
		return nil
	}
	part := string(sd.Content[start:])
	pattern := appearancePrefix
	if listbox {
		pattern = listAppearancePrefix
	}
	match := pattern.FindStringSubmatchIndex(part)
	if match == nil {
		return nil
	}
	fontOperators := preserveRGBDefault(part[match[4]:match[5]], *da)
	if listbox {
		// Only the generated operator prefix is rewritten, never option text.
		// The reference sets fill color only; selection strokes inherit state.
		fontOperators = appearanceStrokeColor.ReplaceAllString(fontOperators, "")
	}
	fontKey := part[match[6]:match[7]]
	fontDict := dictOf(x, dictOf(x, sd.Dict["Resources"]).d["Font"]).d
	font := dictOf(x, fontDict[fontKey]).d
	// Match the reference core-font ascenders, distinct from FontBBox metrics.
	// The reference uses 629 for every Courier style, including bold.
	ascent, supported := map[string]float64{"Times-Roman": 683, "Times-Bold": 683, "Times-Italic": 683, "Times-BoldItalic": 683, "Helvetica": 718, "Helvetica-Bold": 718, "Helvetica-Oblique": 718, "Helvetica-BoldOblique": 718, "Courier": 629, "Courier-Bold": 629, "Courier-Oblique": 629, "Courier-BoldOblique": 629}[nameOf(x, font, "BaseFont")]
	// Core font bounding-box top and height determine multiline baseline and
	// leading. Oblique styles share these vertical metrics with upright styles.
	top, leading := 931.0, 1156.0
	switch nameOf(x, font, "BaseFont") {
	case "Times-Roman":
		top, leading = 898, 1116
	case "Times-BoldItalic":
		top, leading = 921, 1139
	case "Times-Italic":
		top, leading = 883, 1100
	case "Times-Bold":
		top, leading = 935, 1153
	case "Helvetica-Bold", "Helvetica-BoldOblique":
		top, leading = 962, 1190
	case "Courier", "Courier-Oblique":
		top, leading = 805, 1055
	case "Courier-Bold", "Courier-BoldOblique":
		top, leading = 801, 1051
	}
	if !supported || font["FontDescriptor"] != nil {
		return nil
	}
	actualSize, _ := strconv.ParseFloat(part[match[8]:match[9]], 64)
	if !autoSize && actualSize != points {
		return nil
	}
	inset := math.Max(margin, 1)
	if box[2] <= 6*inset || box[3] <= 3*margin {
		return nil
	}
	xOffset := 2 * inset
	if alignment != 0 || multiline || listbox || comb || autoSize {
		// These evidenced Latin-1 glyphs are safe only when the font encoding
		// maps their bytes to the expected names. Keep other Unicode layouts
		// on the existing path until their glyph mapping is verified.
		accentGlyphs := map[rune]string{'É': "Eacute", 'é': "eacute", 'à': "agrave"}
		accentLayout := f.FT == "Tx" && !multiline && !comb && !autoSize && nameOf(x, font, "BaseFont") == "Helvetica"
		usedAccents := map[int]string{}
		widthBytes := []byte{}
		// Require printable ASCII or the explicitly verified accent mappings.
		// Other encodings and Unicode still need independent coverage.
		for _, r := range value {
			if (multiline || listbox) && (r == '\n' || r == '\r') {
				continue
			}
			if r < 32 || r > 126 {
				glyph, ok := accentGlyphs[r]
				if !accentLayout || !ok {
					return nil
				}
				usedAccents[int(r)] = glyph
			}
			widthBytes = append(widthBytes, byte(r))
		}
		verifiedAccents := map[int]bool{}
		if encoding := font["Encoding"]; encoding != nil {
			obj, err := x.Dereference(encoding)
			if err != nil {
				return err
			}
			switch enc := obj.(type) {
			case types.Name:
				if enc == "WinAnsiEncoding" {
					for code := range usedAccents {
						verifiedAccents[code] = true
					}
				}
				if enc != "WinAnsiEncoding" && enc != "StandardEncoding" {
					return nil
				}
			case types.Dict:
				base := nameOf(x, enc, "BaseEncoding")
				if base == "WinAnsiEncoding" {
					for code := range usedAccents {
						verifiedAccents[code] = true
					}
				}
				if base != "" && base != "WinAnsiEncoding" && base != "StandardEncoding" {
					return nil
				}
				diffs, err := x.DereferenceArray(enc["Differences"])
				if err != nil {
					return err
				}
				code := 0
				for _, diff := range diffs {
					switch item := diff.(type) {
					case types.Integer:
						code = int(item)
					case types.Name:
						if strings.ContainsRune(value, rune(code)) {
							if glyph, ok := usedAccents[code]; !ok || string(item) != glyph {
								return nil
							}
							verifiedAccents[code] = true
						}
						code++
					default:
						return nil
					}
				}
			default:
				return nil
			}
		}
		for code := range usedAccents {
			if !verifiedAccents[code] {
				return nil
			}
		}
		if autoSize {
			// Verified single-line Helvetica automatic sizing: fit the width
			// and font bbox height, with a four-point minimum. Keep DA at zero
			// so subsequent edits still request automatic sizing.
			if nameOf(x, font, "BaseFont") != "Helvetica" {
				return nil
			}
			unitWidth, err := pdffont.TextWidthFloat(context.Background(), value, "Helvetica", 1)
			if err != nil {
				return err
			}
			if unitWidth == 0 {
				unitWidth = 1
			}
			points = math.RoundToEven(math.Max(4, math.Min((box[3]-2*margin)/(leading/1000), (box[2]-4*inset)/unitWidth))*10) / 10
			fontOperators = strings.Replace(fontOperators, "/"+fontKey+" "+part[match[8]:match[9]]+" Tf", fmt.Sprintf("/%s %g Tf", fontKey, points), 1)
		}
		// Core-font widths consume encoded bytes, not UTF-8.
		widthValue := value
		if len(usedAccents) > 0 {
			widthValue = string(widthBytes)
		}
		width, err := pdffont.TextWidthFloat(context.Background(), widthValue, nameOf(x, font, "BaseFont"), points)
		if err != nil {
			return err
		}
		if alignment == 1 {
			xOffset = (box[2] - width) / 2
		} else if alignment == 2 {
			xOffset = box[2] - 2*inset - width
		}
		xOffset = math.Round(xOffset*1000) / 1000
	}
	// Preserve the reference pending clipping path through BT. Ending it with
	// n before text changes PDFium edge coverage for bold Courier glyphs.
	prefix := fmt.Sprintf("/Tx BMC q %g %g %g %g re W %s%g %g Td (", 2*inset, margin, box[2]-6*inset, box[3]-3*margin, fontOperators, xOffset, math.Round((box[3]/2-ascent*points/2000)*1000)/1000)
	body := prefix + part[match[1]:]
	if comb {
		// Verified fixed-size Helvetica combs center individual ASCII glyphs
		// in MaxLen cells; only the appearance is truncated, never stored V.
		cells := intOf(x, f.D, "MaxLen")
		if cells <= 0 || nameOf(x, font, "BaseFont") != "Helvetica" {
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "/Tx BMC q %g %g %g %g re W %s", 2*inset, margin, box[2]-6*inset, box[3]-3*margin, fontOperators)
		previousX := 0.0
		for i, glyph := range value {
			if i >= cells {
				break
			}
			width, err := pdffont.TextWidthFloat(context.Background(), string(glyph), "Helvetica", points)
			if err != nil {
				return err
			}
			xPos := (float64(i)+0.5)*box[2]/float64(cells) - width/2
			yPos := 0.0
			if i == 0 {
				yPos = box[3]/2 - ascent*points/2000
			}
			literal := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(string(glyph))
			fmt.Fprintf(&b, "%g %g Td (%s) Tj ", math.Round((xPos-previousX)*1000)/1000, math.Round(yPos*1000)/1000, literal)
			previousX = xPos
		}
		b.WriteString("ET Q EMC ")
		body = b.String()
	} else if multiline || listbox {
		// The reference uses the core font's top bbox for the first
		// baseline and bbox height for leading. Fixed-size text is
		// clipped, not automatically word-wrapped.
		var b strings.Builder
		fmt.Fprintf(&b, "/Tx BMC q %g %g %g %g re W %s", 2*inset, margin, box[2]-6*inset, box[3]-3*margin, fontOperators)
		lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n"), "\n")
		previousX := 0.0
		for i, line := range lines {
			lineX := 2 * inset
			if alignment != 0 {
				width, err := pdffont.TextWidthFloat(context.Background(), line, nameOf(x, font, "BaseFont"), points)
				if err != nil {
					return err
				}
				if alignment == 1 {
					lineX = (box[2] - width) / 2
				} else {
					lineX = box[2] - 2*inset - width
				}
			}
			xPos, yPos := lineX-previousX, -points*leading/1000
			if i == 0 {
				yPos = box[3] - margin - points*top/1000
				if listbox {
					yPos = box[3]/2 - ascent*points/2000
				}
			}
			if listbox && selected != "" && strings.Contains(selected, line) {
				y := box[3]/2 - ascent*points/2000 - float64(i)*points*leading/1000 - 1
				fmt.Fprintf(&b, "1 %g %g %g re 0.5 0.5 0.5 rg s %s", math.Round(y*1000)/1000, box[2]-2, points+2, strings.TrimPrefix(fontOperators, "BT "))
			}
			previousX = lineX
			literal := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(line)
			fmt.Fprintf(&b, "%g %g Td (%s) Tj ", math.Round(xPos*1000)/1000, math.Round(yPos*1000)/1000, literal)
		}
		b.WriteString("ET Q EMC ")
		body = b.String()
	}
	sd.Content = append(append([]byte{}, sd.Content[:start]...), []byte(body)...)
	if err := sd.Encode(); err != nil {
		return err
	}
	ref, err := x.IndRefForNewObject(*sd)
	if err != nil {
		return err
	}
	ap["N"] = *ref
	return nil
}
