package pdf

import (
	"fmt"
	"image"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/structs"
)

// renderOpaque uses the reference preview's white, opaque BGR bitmap. Drawing
// forms into an alpha bitmap changes antialiasing even after compositing white.
func renderOpaque(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT, page requests.Page, width, height int) (*image.RGBA, error) {
	bitmap, err := inst.FPDFBitmap_CreateEx(&requests.FPDFBitmap_CreateEx{Width: width, Height: height, Format: enums.FPDF_BITMAP_FORMAT_BGR, Pointer: uint64(0)})
	if err != nil {
		return nil, err
	}
	defer inst.FPDFBitmap_Destroy(&requests.FPDFBitmap_Destroy{Bitmap: bitmap.Bitmap})
	_, err = inst.FPDFBitmap_FillRect(&requests.FPDFBitmap_FillRect{Bitmap: bitmap.Bitmap, Width: width, Height: height, Color: 0xffffffff})
	if err != nil {
		return nil, err
	}
	// Static previewing never runs document actions, timers, or JavaScript.
	form, err := inst.FPDFDOC_InitFormFillEnvironment(&requests.FPDFDOC_InitFormFillEnvironment{Document: doc, FormFillInfo: structs.FPDF_FORMFILLINFO{
		FFI_Invalidate:         func(references.FPDF_PAGE, float64, float64, float64, float64) {},
		FFI_SetCursor:          func(enums.FXCT) {},
		FFI_SetTimer:           func(int, func(int)) int { return 0 },
		FFI_KillTimer:          func(int) {},
		FFI_GetLocalTime:       func() structs.FPDF_SYSTEMTIME { return structs.FPDF_SYSTEMTIME{} },
		FFI_GetPage:            func(references.FPDF_DOCUMENT, int) *references.FPDF_PAGE { return nil },
		FFI_GetRotation:        func(references.FPDF_PAGE) enums.FPDF_PAGE_ROTATION { return 0 },
		FFI_ExecuteNamedAction: func(string) {},
	}})
	if err != nil {
		return nil, err
	}
	defer inst.FPDFDOC_ExitFormFillEnvironment(&requests.FPDFDOC_ExitFormFillEnvironment{FormHandle: form.FormHandle})
	_, err = inst.FORM_OnAfterLoadPage(&requests.FORM_OnAfterLoadPage{Page: page, FormHandle: form.FormHandle})
	if err != nil {
		return nil, err
	}
	defer inst.FORM_OnBeforeClosePage(&requests.FORM_OnBeforeClosePage{Page: page, FormHandle: form.FormHandle})
	// Include non-interactive annotations, as the reference page preview does.
	_, err = inst.FPDF_RenderPageBitmap(&requests.FPDF_RenderPageBitmap{Bitmap: bitmap.Bitmap, Page: page, SizeX: width, SizeY: height, Flags: enums.FPDF_RENDER_FLAG_ANNOT})
	if err != nil {
		return nil, err
	}
	_, err = inst.FPDF_FFLDraw(&requests.FPDF_FFLDraw{Bitmap: bitmap.Bitmap, Page: page, FormHandle: form.FormHandle, SizeX: width, SizeY: height})
	if err != nil {
		return nil, err
	}
	stride, err := inst.FPDFBitmap_GetStride(&requests.FPDFBitmap_GetStride{Bitmap: bitmap.Bitmap})
	if err != nil {
		return nil, err
	}
	buf, err := inst.FPDFBitmap_GetBuffer(&requests.FPDFBitmap_GetBuffer{Bitmap: bitmap.Bitmap})
	if err != nil {
		return nil, err
	}
	return opaqueBGR(buf.Buffer, stride.Stride, width, height)
}

func opaqueBGR(buf []byte, stride, width, height int) (*image.RGBA, error) {
	if width <= 0 || height <= 0 || stride < 0 || width > stride/3 || height > len(buf)/max(1, stride) {
		return nil, fmt.Errorf("invalid PDF bitmap dimensions or buffer")
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			s, d := y*stride+x*3, y*dst.Stride+x*4
			dst.Pix[d], dst.Pix[d+1], dst.Pix[d+2], dst.Pix[d+3] = buf[s+2], buf[s+1], buf[s], 255
		}
	}
	return dst, nil
}
