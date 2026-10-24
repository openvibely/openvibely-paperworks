package pdf

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// PDFFont describes font metadata used by page text objects, including nested
// Form XObjects. It does not enumerate unused resources or annotation appearances.
type PDFFont struct {
	Name            string `json:"name"`
	NameUnavailable bool   `json:"name_unavailable,omitempty"`
	TextObject      int    `json:"text_object,omitempty"`
	Embedded        bool   `json:"embedded"`
	Pages           []int  `json:"pages"`
}

func ListFonts(ctx context.Context, data []byte) ([]PDFFont, error) {
	inst, done, err := engine(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, err
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, err
	}
	type key struct {
		name     string
		embedded bool
	}
	seen := map[key]int{}
	out := []PDFFont{}
	visited := 0
	for p := 0; p < count.PageCount; p++ {
		err = func() error {
			page, e := inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc.Document, Index: p})
			if e != nil {
				return e
			}
			defer inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page})
			ref := requests.Page{ByReference: &page.Page}
			n, e := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: ref})
			if e != nil {
				return e
			}
			if n.Count > 100000 {
				return fmt.Errorf("font inspection object limit exceeded")
			}
			var pending []references.FPDF_PAGEOBJECT
			for i := 0; i < n.Count; i++ {
				o, e := inst.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: ref, Index: i})
				if e != nil {
					return e
				}
				pending = append(pending, o.PageObject)
			}
			for len(pending) > 0 {
				if e := ctx.Err(); e != nil {
					return e
				}
				visited++
				if visited > 100000 || len(pending) > 100000 {
					return fmt.Errorf("font inspection object limit exceeded")
				}
				o := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				typ, e := inst.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: o})
				if e != nil {
					return e
				}
				switch typ.Type {
				case enums.FPDF_PAGEOBJ_FORM:
					n, e := inst.FPDFFormObj_CountObjects(&requests.FPDFFormObj_CountObjects{PageObject: o})
					if e != nil {
						return e
					}
					if n.Count > 100000-len(pending) {
						return fmt.Errorf("font inspection object limit exceeded")
					}
					for j := 0; j < n.Count; j++ {
						child, e := inst.FPDFFormObj_GetObject(&requests.FPDFFormObj_GetObject{PageObject: o, Index: uint64(j)})
						if e != nil {
							return e
						}
						pending = append(pending, child.PageObject)
					}
				case enums.FPDF_PAGEOBJ_TEXT:
					f, e := inst.FPDFTextObj_GetFont(&requests.FPDFTextObj_GetFont{PageObject: o})
					if e != nil {
						return e
					}
					name, e := inst.FPDFFont_GetBaseFontName(&requests.FPDFFont_GetBaseFontName{Font: f.Font})
					if e != nil {
						return e
					}
					embedded, e := inst.FPDFFont_GetIsEmbedded(&requests.FPDFFont_GetIsEmbedded{Font: f.Font})
					if e != nil {
						return e
					}
					// PDFium's Go wrapper creates a new font handle for each lookup,
					// so it cannot establish identity for unnamed fonts. Keep each
					// use separate rather than merging unrelated font programs.
					if strings.TrimSpace(name.BaseFontName) == "" {
						out = append(out, PDFFont{Name: "unavailable", NameUnavailable: true, TextObject: visited, Embedded: embedded.IsEmbedded, Pages: []int{p + 1}})
						continue
					}
					k := key{name.BaseFontName, embedded.IsEmbedded}
					idx, ok := seen[k]
					if !ok {
						idx = len(out)
						seen[k] = idx
						out = append(out, PDFFont{Name: k.name, Embedded: k.embedded, Pages: []int{}})
					}
					if !slices.Contains(out[idx].Pages, p+1) {
						out[idx].Pages = append(out[idx].Pages, p+1)
					}
				}
			}
			return nil
		}()
		if err != nil {
			return nil, fmt.Errorf("inspect fonts on page %d: %w", p+1, err)
		}
	}
	slices.SortFunc(out, func(a, b PDFFont) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		if a.Embedded == b.Embedded {
			return a.TextObject - b.TextObject
		}
		if a.Embedded {
			return 1
		}
		return -1
	})
	return out, nil
}
