package pdf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/primitives"
	"golang.org/x/image/font/sfnt"
)

// CreateSpec provides fixed-layout PDF authoring in points with a bottom-left
// origin. Native form elements remain interactive. Flow content is mutually
// exclusive with explicitly positioned pages.
type CreateSpec struct {
	Canvas *Canvas      `json:"canvas,omitempty"`
	Paper  string       `json:"paper"`
	Pages  []CreatePage `json:"pages"`
	Flow   *FlowSpec    `json:"flow,omitempty"`
}
type CreatePage struct {
	FixedParagraphs []FixedParagraph `json:"fixed_paragraphs,omitempty"`
	FixedSpacers    [][2]float64     `json:"fixed_spacers,omitempty"`
	// Size overrides final page boxes in points without scaling fixed content.
	Size        *[2]float64                    `json:"size,omitempty"`
	FixedTables []FixedTable                   `json:"fixed_tables,omitempty"`
	Paper       string                         `json:"paper"`
	Text        []*primitives.TextBox          `json:"text"`
	Tables      []*primitives.Table            `json:"tables"`
	Images      []*primitives.ImageBox         `json:"images"`
	TextFields  []*primitives.TextField        `json:"text_fields"`
	Checkboxes  []*primitives.CheckBox         `json:"checkboxes"`
	RadioGroups []*primitives.RadioButtonGroup `json:"radio_groups"`
	ComboBoxes  []*primitives.ComboBox         `json:"combo_boxes"`
}

func cloneTextFieldLabel(label *primitives.TextFieldLabel) *primitives.TextFieldLabel {
	if label == nil {
		return nil
	}
	cloned := *label
	return &cloned
}

func cloneCreatePageForms(page CreatePage) CreatePage {
	if page.TextFields != nil {
		fields := make([]*primitives.TextField, len(page.TextFields))
		for i, field := range page.TextFields {
			if field != nil {
				cloned := *field
				cloned.Label = cloneTextFieldLabel(field.Label)
				fields[i] = &cloned
			}
		}
		page.TextFields = fields
	}
	if page.ComboBoxes != nil {
		fields := make([]*primitives.ComboBox, len(page.ComboBoxes))
		for i, field := range page.ComboBoxes {
			if field != nil {
				cloned := *field
				cloned.Label = cloneTextFieldLabel(field.Label)
				fields[i] = &cloned
			}
		}
		page.ComboBoxes = fields
	}
	if page.Checkboxes != nil {
		fields := make([]*primitives.CheckBox, len(page.Checkboxes))
		for i, field := range page.Checkboxes {
			if field != nil {
				cloned := *field
				cloned.Label = cloneTextFieldLabel(field.Label)
				fields[i] = &cloned
			}
		}
		page.Checkboxes = fields
	}
	if page.RadioGroups != nil {
		fields := make([]*primitives.RadioButtonGroup, len(page.RadioGroups))
		for i, field := range page.RadioGroups {
			if field != nil {
				cloned := *field
				cloned.Label = cloneTextFieldLabel(field.Label)
				if field.Buttons != nil {
					buttons := *field.Buttons
					buttons.Label = cloneTextFieldLabel(field.Buttons.Label)
					cloned.Buttons = &buttons
				}
				fields[i] = &cloned
			}
		}
		page.RadioGroups = fields
	}
	return page
}

func supportedText(s string) error {
	for _, r := range s {
		if (r < 32 && r != '\n' && r != '\r' && r != '\t') || r > 126 {
			return fmt.Errorf("PDF authoring supports printable ASCII only outside interactive form text (unsupported character U+%04X)", r)
		}
	}
	return nil
}

func Create(ctx context.Context, spec CreateSpec, base, output string) error {
	if spec.Canvas != nil {
		if len(spec.Pages) > 0 || spec.Flow != nil {
			return fmt.Errorf("canvas, pages and flow are mutually exclusive")
		}
		return createCanvasPDF(ctx, *spec.Canvas, base, output)
	}
	if spec.Paper == "" {
		spec.Paper = "A4"
	}
	if spec.Flow != nil {
		if len(spec.Pages) > 0 {
			return fmt.Errorf("choose pages or flow, not both")
		}
		var err error
		spec.Pages, err = flowPages(ctx, spec.Paper, spec.Flow)
		if err != nil {
			return err
		}
	}
	if len(spec.Pages) < 1 || len(spec.Pages) > 1000 {
		return fmt.Errorf("pages must contain 1..1000 entries")
	}
	if spec.Paper == "" {
		spec.Paper = "A4"
	}
	checkFont := func(f *primitives.FormFont) error {
		if f == nil {
			return nil
		}
		switch f.Name {
		case "", "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique", "Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic", "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique", "Symbol", "ZapfDingbats":
			return nil
		}
		return fmt.Errorf("unsupported PDF font %q", f.Name)
	}
	unicodeFont, unicodeFontName, err := formUnicodeFont()
	if err != nil {
		return err
	}
	fontBuffer := &sfnt.Buffer{}
	usesUnicodeFont := false
	validateFormText := func(s string) (bool, error) {
		return validateFormText(s, unicodeFont, unicodeFontName, fontBuffer)
	}
	checkLabel := func(label *primitives.TextFieldLabel) (bool, error) {
		if label == nil {
			return false, nil
		}
		unicodeText, err := validateFormText(label.Value)
		if err != nil {
			return false, err
		}
		if err := checkFont(label.Font); err != nil {
			return false, err
		}
		if unicodeText {
			usesUnicodeFont = true
			withFormUnicodeFont(&label.Font, unicodeFontName)
		}
		return unicodeText, nil
	}
	doc := &primitives.PDF{Paper: spec.Paper, Origin: "ll", Fonts: map[string]*primitives.FormFont{"input": {Name: "Helvetica", Size: 12}, "label": {Name: "Helvetica", Size: 12}, "text": {Name: "Helvetica", Size: 12}}, Pages: map[string]*primitives.PDFPage{}}
	imageRoot := ""
	defer func() {
		if imageRoot != "" {
			_ = os.RemoveAll(imageRoot)
		}
	}()
	elements := 0
	for i, p := range spec.Pages {
		p = cloneCreatePageForms(p)
		if e := ctx.Err(); e != nil {
			return e
		}
		if p.Size != nil && (!finite(p.Size[0]) || !finite(p.Size[1]) || p.Size[0] <= 0 || p.Size[1] <= 0 || p.Size[0] > 14400 || p.Size[1] > 14400) {
			return fmt.Errorf("page size must be finite and in (0,14400] points")
		}
		for _, paragraph := range p.FixedParagraphs {
			if err := paragraph.validate(ctx); err != nil {
				return err
			}
		}
		for _, pos := range p.FixedSpacers {
			if !finite(pos[0]) || !finite(pos[1]) {
				return fmt.Errorf("invalid spacer position")
			}
		}
		for _, table := range p.FixedTables {
			if err := table.validate(); err != nil {
				return err
			}
		}
		elements += len(p.FixedParagraphs) + len(p.FixedSpacers) + len(p.FixedTables) + len(p.Text) + len(p.Tables) + len(p.Images) + len(p.TextFields) + len(p.Checkboxes) + len(p.RadioGroups) + len(p.ComboBoxes)
		if elements > 10000 {
			return fmt.Errorf("document exceeds 10000 elements")
		}
		for _, v := range p.Text {
			if v == nil {
				return fmt.Errorf("null text element")
			}
			if e := supportedText(v.Value); e != nil {
				return e
			}
			if e := checkFont(v.Font); e != nil {
				return e
			}
		}
		for _, v := range p.Tables {
			if v == nil {
				return fmt.Errorf("null table")
			}
			for _, row := range v.Values {
				for _, text := range row {
					if e := supportedText(text); e != nil {
						return e
					}
				}
			}
			if v.Header != nil {
				for _, text := range v.Header.Values {
					if e := supportedText(text); e != nil {
						return e
					}
				}
			}
			if e := checkFont(v.Font); e != nil {
				return e
			}
			if v.Header != nil {
				if e := checkFont(v.Header.Font); e != nil {
					return e
				}
			}
			if v.Rows*v.Cols > 100000 || v.Rows < 0 || v.Cols < 0 || len(v.Values) > 10000 {
				return fmt.Errorf("table exceeds limits")
			}
		}
		for _, v := range p.TextFields {
			if v == nil {
				return fmt.Errorf("null text field")
			}
			if _, err := checkLabel(v.Label); err != nil {
				return err
			}
			valueUnicode := false
			for _, text := range []string{v.Value, v.Default} {
				unicodeText, err := validateFormText(text)
				if err != nil {
					return err
				}
				valueUnicode = valueUnicode || unicodeText
			}
			if err := checkFont(v.Font); err != nil {
				return err
			}
			if valueUnicode {
				usesUnicodeFont = true
				withFormUnicodeFont(&v.Font, unicodeFontName)
			}
		}
		for _, v := range p.ComboBoxes {
			if v == nil {
				return fmt.Errorf("null combo box")
			}
			valueUnicode := false
			for _, text := range append([]string{v.Value, v.Default}, v.Options...) {
				unicodeText, err := validateFormText(text)
				if err != nil {
					return err
				}
				valueUnicode = valueUnicode || unicodeText
			}
			if _, err := checkLabel(v.Label); err != nil {
				return err
			}
			if err := checkFont(v.Font); err != nil {
				return err
			}
			if valueUnicode {
				usesUnicodeFont = true
				withFormUnicodeFont(&v.Font, unicodeFontName)
			}
		}
		for _, v := range p.Checkboxes {
			if v == nil {
				return fmt.Errorf("null checkbox")
			}
			if _, err := checkLabel(v.Label); err != nil {
				return err
			}
		}
		for _, v := range p.RadioGroups {
			if v == nil {
				return fmt.Errorf("null radio group")
			}
			if v.Buttons != nil {
				if _, err := checkLabel(v.Buttons.Label); err != nil {
					return err
				}
				for _, text := range v.Buttons.Values {
					if err := supportedText(text); err != nil {
						return err
					}
				}
			}
			if _, err := checkLabel(v.Label); err != nil {
				return err
			}
		}
		p.Images = append([]*primitives.ImageBox(nil), p.Images...)
		for j, v := range p.Images {
			if v == nil {
				return fmt.Errorf("null image")
			}
			if strings.Contains(v.Src, "://") || strings.HasPrefix(v.Src, "//") || strings.HasPrefix(v.Src, `\\`) {
				return fmt.Errorf("image must be a local asset path")
			}
			if !filepath.IsAbs(v.Src) {
				v.Src = filepath.Join(base, v.Src)
			}
			info, e := os.Stat(v.Src)
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() || info.Size() > 32<<20 {
				return fmt.Errorf("image must be a regular file of at most 32 MiB")
			}
			// Give pdfcpu access only to explicitly supplied image assets.
			if imageRoot == "" {
				imageRoot, e = os.MkdirTemp("", "paperworks-pdf-images-*")
				if e != nil {
					return e
				}
			}
			data, e := os.ReadFile(v.Src)
			if e != nil {
				return e
			}
			if len(data) > 32<<20 {
				return fmt.Errorf("image must be at most 32 MiB")
			}
			image := *v
			image.Src = fmt.Sprintf("page-%d-image-%d%s", i, j, filepath.Ext(v.Src))
			if e = os.WriteFile(filepath.Join(imageRoot, image.Src), data, 0600); e != nil {
				return e
			}
			p.Images[j] = &image
		}
		doc.Pages[strconv.Itoa(i+1)] = &primitives.PDFPage{Paper: p.Paper, Content: &primitives.Content{TextBoxes: p.Text, Tables: p.Tables, ImageBoxes: p.Images, TextFields: p.TextFields, CheckBoxes: p.Checkboxes, RadioButtonGroups: p.RadioGroups, ComboBoxes: p.ComboBoxes}}
	}
	data, e := json.Marshal(map[string]any{"paper": doc.Paper, "origin": doc.Origin, "fonts": doc.Fonts, "pages": doc.Pages})
	if e != nil {
		return e
	}
	conf := config()
	if usesUnicodeFont {
		fontRoot, err := os.MkdirTemp("", "paperworks-form-fonts-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(fontRoot)
		conf, err = model.LoadConfiguration(fontRoot)
		if err != nil {
			return err
		}
		fontDir, _ := conf.UserFontStore()
		if err := installFormUnicodeFont(fontDir); err != nil {
			return err
		}
	}
	conf.Offline = true
	conf.LocalImageRoot = imageRoot
	var out bytes.Buffer
	if e = api.Create(ctx, nil, bytes.NewReader(data), &out, conf); e != nil {
		return e
	}
	c, e := api.ReadAndValidate(ctx, bytes.NewReader(out.Bytes()), conf)
	if e != nil {
		return fmt.Errorf("validate generated PDF: %w", e)
	}
	if e = c.EnsurePageCount(); e != nil {
		return fmt.Errorf("validate generated PDF: %w", e)
	}
	// pdfcpu creates choice widgets without a normal appearance. Materialize
	// their saved values before publishing so viewers need not regenerate them.
	fields, _, e := fieldTree(c.XRefTable)
	if e != nil {
		return e
	}
	choices := map[string]string{}
	for name, f := range fields {
		if f.FT == "Ch" {
			value := ""
			if v := display(c.XRefTable, f.D["V"]); v != nil {
				value = *v
			}
			choices[name] = value
		}
	}
	changed, e := applyFixedLayout(ctx, c, spec.Pages)
	if e != nil {
		return e
	}
	data = out.Bytes()
	if changed {
		if data, e = Serialize(ctx, c); e != nil {
			return e
		}
	}
	if len(choices) > 0 {
		if e = applyValuesWithFontRefresh(ctx, c, fields, choices, usesUnicodeFont, nil); e != nil {
			return e
		}
		if data, e = Serialize(ctx, c); e != nil {
			return e
		}
		report, err := ReportBytes(ctx, data)
		if err != nil {
			return err
		}
		if len(report.Issues) > 0 {
			return fmt.Errorf("validate generated form: %s", strings.Join(report.Issues, "; "))
		}
	}

	if report, ok, err := reportDocument(ctx, c, spec.Pages); err != nil {
		return err
	} else if ok {
		data = report
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return opc.WriteNew(output, data)
}
