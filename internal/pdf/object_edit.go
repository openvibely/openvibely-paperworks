package pdf

import (
	"bytes"
	"context"
	"fmt"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/structs"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"math"
	"os"
	"path/filepath"
	"sort"
)

type ObjectInfo struct {
	Page   int        `json:"page"`
	Index  int        `json:"index"`
	Type   int        `json:"type"`
	Bounds [4]float32 `json:"bounds"`
}
type ObjectEditSpec struct {
	Edits []ObjectEdit `json:"edits"`
}
type ObjectEdit struct {
	Page        int         `json:"page"`
	Remove      []int       `json:"remove,omitempty"`
	OverlayPDF  string      `json:"overlay_pdf,omitempty"`
	OverlayPage int         `json:"overlay_page,omitempty"`
	Matrix      *[6]float64 `json:"matrix,omitempty"`
}

func Objects(ctx context.Context, data []byte) ([]ObjectInfo, error) {
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
	out := []ObjectInfo{}
	for i := 0; i < count.PageCount; i++ {
		page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}}
		n, err := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
		if err != nil {
			return nil, err
		}
		for j := 0; j < n.Count; j++ {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			if len(out) >= 100000 {
				return nil, fmt.Errorf("object limit exceeded")
			}
			obj, err := inst.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: j})
			if err != nil {
				return nil, err
			}
			typ, err := inst.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: obj.PageObject})
			if err != nil {
				return nil, err
			}
			b, err := inst.FPDFPageObj_GetBounds(&requests.FPDFPageObj_GetBounds{PageObject: obj.PageObject})
			if err != nil {
				return nil, err
			}
			out = append(out, ObjectInfo{i + 1, j, int(typ.Type), [4]float32{b.Left, b.Bottom, b.Right, b.Top}})
		}
	}
	return out, nil
}

// EditObjects removes selected top-level objects and optionally imports another
// PDF page as vector artwork. It never rasterizes untouched page content.
func EditObjects(ctx context.Context, input, output, base string, s ObjectEditSpec) error {
	if len(s.Edits) == 0 || len(s.Edits) > 1000 {
		return fmt.Errorf("edits must contain 1..1000 operations")
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	inst, done, err := engine(ctx)
	if err != nil {
		return err
	}
	defer done()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return err
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return err
	}
	var resources []func()
	defer func() {
		for i := len(resources) - 1; i >= 0; i-- {
			resources[i]()
		}
	}()
	for _, e := range s.Edits {
		if err = ctx.Err(); err != nil {
			return err
		}
		if e.Page < 1 || e.Page > count.PageCount {
			return fmt.Errorf("page out of range")
		}
		page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: e.Page - 1}}
		n, err := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
		if err != nil {
			return err
		}
		ids := append([]int(nil), e.Remove...)
		sort.Sort(sort.Reverse(sort.IntSlice(ids)))
		for i, id := range ids {
			if id < 0 || id >= n.Count || (i > 0 && ids[i-1] == id) {
				return fmt.Errorf("invalid or duplicate object index")
			}
			obj, err := inst.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: id})
			if err != nil {
				return err
			}
			if _, err = inst.FPDFPage_RemoveObject(&requests.FPDFPage_RemoveObject{Page: page, PageObject: obj.PageObject}); err != nil {
				return err
			}
			if _, err = inst.FPDFPageObj_Destroy(&requests.FPDFPageObj_Destroy{PageObject: obj.PageObject}); err != nil {
				return err
			}
		}
		if e.OverlayPDF != "" {
			err = func() error {
				file := e.OverlayPDF
				if !filepath.IsAbs(file) {
					file = filepath.Join(base, file)
				}
				raw, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				source, err := inst.OpenDocument(&requests.OpenDocument{File: &raw})
				if err != nil {
					return err
				}
				resources = append(resources, func() { inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: source.Document}) })
				p := e.OverlayPage
				if p == 0 {
					p = 1
				}
				if p < 1 {
					return fmt.Errorf("overlay_page must be positive")
				}
				// go-pdfium v1.21.3 forwards Source first, but PDFium's C API
				// takes (dest_doc, src_doc, page). Compensate for that pinned
				// binding; TestReplaceArtworkPreservesText covers saved output.
				// https://pdfium.googlesource.com/pdfium/+/refs/heads/main/public/fpdf_ppo.h
				x, err := inst.FPDF_NewXObjectFromPage(&requests.FPDF_NewXObjectFromPage{Source: doc.Document, Destination: source.Document, SourcePageIndex: p - 1})
				if err != nil {
					return err
				}
				resources = append(resources, func() { inst.FPDF_CloseXObject(&requests.FPDF_CloseXObject{XObject: x.XObject}) })
				obj, err := inst.FPDF_NewFormObjectFromXObject(&requests.FPDF_NewFormObjectFromXObject{XObject: x.XObject})
				if err != nil {
					return err
				}
				owned := true
				defer func() {
					if owned {
						inst.FPDFPageObj_Destroy(&requests.FPDFPageObj_Destroy{PageObject: obj.PageObject})
					}
				}()
				m := [6]float64{1, 0, 0, 1, 0, 0}
				if e.Matrix != nil {
					m = *e.Matrix
				}
				for _, v := range m {
					if !finite(v) || math.Abs(v) > 1e8 {
						return fmt.Errorf("invalid transformation matrix")
					}
				}
				if _, err = inst.FPDFPageObj_Transform(&requests.FPDFPageObj_Transform{PageObject: obj.PageObject, Transform: structs.FPDF_FS_MATRIX{A: float32(m[0]), B: float32(m[1]), C: float32(m[2]), D: float32(m[3]), E: float32(m[4]), F: float32(m[5])}}); err != nil {
					return err
				}
				if _, err = inst.FPDFPage_InsertObject(&requests.FPDFPage_InsertObject{Page: page, PageObject: obj.PageObject}); err != nil {
					return err
				}
				owned = false
				return nil
			}()
			if err != nil {
				return err
			}
		}
		if _, err = inst.FPDFPage_GenerateContent(&requests.FPDFPage_GenerateContent{Page: page}); err != nil {
			return err
		}
	}
	var b bytes.Buffer
	if _, err = inst.FPDF_SaveAsCopy(&requests.FPDF_SaveAsCopy{Document: doc.Document, FileWriter: &b}); err != nil {
		return err
	}
	return opc.WriteNew(output, b.Bytes())
}
