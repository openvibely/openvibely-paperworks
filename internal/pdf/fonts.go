package pdf

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

type FontUse struct {
	Family string `json:"family"`
	Pages  []int  `json:"pages"`
}

var (
	subsetRe = regexp.MustCompile(`^[A-Z]{6}\+`)
	styleRe  = regexp.MustCompile(`[-,](?:BoldItalic|BoldOblique|Bold|Italic|Oblique|Roman|Regular)(?:MT)?$`)
)

func normFamily(s string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s))
}

// VerifyFonts checks that every visible text object uses an approved font
// family, reading the family from the font program when available.
func VerifyFonts(ctx context.Context, data []byte, families []string) ([]FontUse, error) {
	approved := map[string]bool{}
	for _, f := range families {
		if strings.TrimSpace(f) == "" {
			return nil, errors.New("rendered-font verification requires nonempty approved font families")
		}
		approved[normFamily(f)] = true
	}
	if len(approved) == 0 {
		return nil, errors.New("rendered-font verification requires nonempty approved font families")
	}
	inst, done, err := engine(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, err
	}
	observed := map[string][]int{}
	var unknown []int
	for i := 0; i < count.PageCount; i++ {
		page, err := inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc.Document, Index: i})
		if err != nil {
			return nil, err
		}
		text, err := inst.FPDFText_LoadPage(&requests.FPDFText_LoadPage{Page: requests.Page{ByReference: &page.Page}})
		if err != nil {
			inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})
			return nil, err
		}
		n, _ := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: requests.Page{ByReference: &page.Page}})
		var objs []references.FPDF_PAGEOBJECT
		for j := 0; n != nil && j < n.Count; j++ {
			o, err := inst.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: requests.Page{ByReference: &page.Page}, Index: j})
			if err == nil {
				objs = append(objs, o.PageObject)
			}
		}
		for len(objs) > 0 {
			o := objs[0]
			objs = objs[1:]
			t, err := inst.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: o})
			if err != nil {
				continue
			}
			switch t.Type {
			case enums.FPDF_PAGEOBJ_FORM:
				c, _ := inst.FPDFFormObj_CountObjects(&requests.FPDFFormObj_CountObjects{PageObject: o})
				for k := 0; c != nil && k < c.Count; k++ {
					if s, err := inst.FPDFFormObj_GetObject(&requests.FPDFFormObj_GetObject{PageObject: o, Index: uint64(k)}); err == nil {
						objs = append(objs, s.PageObject)
					}
				}
			case enums.FPDF_PAGEOBJ_TEXT:
				s, err := inst.FPDFTextObj_GetText(&requests.FPDFTextObj_GetText{PageObject: o, TextPage: text.TextPage})
				if err != nil || strings.TrimSpace(s.Text) == "" {
					continue
				}
				family := objectFamily(inst, o)
				if family == "" {
					unknown = append(unknown, i+1)
					continue
				}
				if !slices.Contains(observed[family], i+1) {
					observed[family] = append(observed[family], i+1)
				}
			}
		}
		inst.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: text.TextPage})
		inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})
	}
	if len(unknown) > 0 || len(observed) == 0 {
		slices.Sort(unknown)
		return nil, fmt.Errorf("cannot establish fonts for rendered text on pages %v", slices.Compact(unknown))
	}
	var out []FontUse
	var unexpected []string
	for f, pages := range observed {
		out = append(out, FontUse{f, pages})
		if !approved[normFamily(f)] {
			unexpected = append(unexpected, f)
		}
	}
	slices.SortFunc(out, func(a, b FontUse) int { return strings.Compare(a.Family, b.Family) })
	if len(unexpected) > 0 {
		slices.Sort(unexpected)
		return nil, errors.New("Unapproved rendered fonts (possible substitution): " + strings.Join(unexpected, ", "))
	}
	return out, nil
}

func objectFamily(inst pdfium.Pdfium, o references.FPDF_PAGEOBJECT) string {
	f, err := inst.FPDFTextObj_GetFont(&requests.FPDFTextObj_GetFont{PageObject: o})
	if err != nil {
		return ""
	}
	// Standard PDF fonts have no embedded program. PDFium's family may name
	// a local substitute (e.g. Arial for Helvetica), unlike Reference's BaseFont.
	baseName, baseErr := inst.FPDFFont_GetBaseFontName(&requests.FPDFFont_GetBaseFontName{Font: f.Font})
	embedded, embeddedErr := inst.FPDFFont_GetIsEmbedded(&requests.FPDFFont_GetIsEmbedded{Font: f.Font})
	if baseErr == nil && embeddedErr == nil && !embedded.IsEmbedded {
		switch baseName.BaseFontName {
		case "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique",
			"Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic",
			"Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique", "Symbol", "ZapfDingbats":
			return styleRe.ReplaceAllString(baseName.BaseFontName, "")
		}
	}
	if fam, err := inst.FPDFFont_GetFamilyName(&requests.FPDFFont_GetFamilyName{Font: f.Font}); err == nil && strings.TrimSpace(fam.FamilyName) != "" {
		return styleRe.ReplaceAllString(subsetRe.ReplaceAllString(fam.FamilyName, ""), "")
	}
	base, err := inst.FPDFFont_GetBaseFontName(&requests.FPDFFont_GetBaseFontName{Font: f.Font})
	if err != nil {
		return ""
	}
	return styleRe.ReplaceAllString(subsetRe.ReplaceAllString(base.BaseFontName, ""), "")
}
