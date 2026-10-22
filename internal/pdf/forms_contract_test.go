package pdf

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/structs"
)

// Test-only PDFium LCD mode matches the original test images() helper.
func referenceRenderLCD(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT, page requests.Page, width, height int) (*image.RGBA, error) {
	return referenceRenderFormPage(inst, doc, page, width, height, enums.FPDF_RENDER_FLAG_ANNOT|enums.FPDF_RENDER_FLAG_LCD_TEXT)
}

// pypdfium2 page.render defaults to draw_annots=True. Form rendering is a
// separate FFLDraw call; flags must be applied to both, as in the pinned helper.
func referenceRenderFormPage(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT, page requests.Page, width, height int, flags enums.FPDF_RENDER_FLAG) (*image.RGBA, error) {
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
	_, err = inst.FPDF_RenderPageBitmap(&requests.FPDF_RenderPageBitmap{Bitmap: bitmap.Bitmap, Page: page, SizeX: width, SizeY: height, Flags: flags})
	if err != nil {
		return nil, err
	}
	_, err = inst.FPDF_FFLDraw(&requests.FPDF_FFLDraw{Bitmap: bitmap.Bitmap, Page: page, FormHandle: form.FormHandle, SizeX: width, SizeY: height, Flags: flags})
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

func referenceFormSource(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("../../testdata/reference-mirror/form.pdf")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func referenceFormMutation(t *testing.T, change func(*model.Context)) []byte {
	t.Helper()
	c, e := Load(context.Background(), referenceFormSource(t))
	if e != nil {
		t.Fatal(e)
	}
	change(c)
	b, e := Serialize(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func referenceWidgets(t *testing.T, c *model.Context, page int) []types.Dict {
	t.Helper()
	w, e := pageWidgets(context.Background(), c, page)
	if e != nil {
		t.Fatal(e)
	}
	return w
}
func referenceFormUnchanged(t *testing.T, src string, before []byte) {
	t.Helper()
	after, e := os.ReadFile(src)
	if e != nil || string(after) != string(before) {
		t.Fatal("source changed", e)
	}
}
func referenceFormPixels(t *testing.T, path, kind string) {
	t.Helper()
	var oracle struct {
		SourceSHA256 string `json:"source_sha256"`
		Cases        map[string][]struct {
			Width, Height int
		}
	}
	b, e := os.ReadFile("../../testdata/reference-mirror/form-oracles.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &oracle); e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(referenceFormSource(t))) != oracle.SourceSHA256 {
		t.Fatal("oracle source changed")
	}
	expected, ok := oracle.Cases[kind]
	if !ok {
		t.Fatal("missing oracle", kind)
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	inst, done, e := engine(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer done()
	doc, e := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if e != nil {
		t.Fatal(e)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	// Render the fixed, previously verified PDF with the same host fonts as
	// the result. Unembedded standard fonts vary across operating systems.
	golden, e := os.ReadFile("testdata/form-appearances/" + kind + ".pdf")
	if e != nil {
		t.Fatal(e)
	}
	ref, e := inst.OpenDocument(&requests.OpenDocument{File: &golden})
	if e != nil {
		t.Fatal(e)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: ref.Document})
	count, e := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if e != nil {
		t.Fatal(e)
	}
	if count.PageCount != 2 {
		t.Fatalf("page count %d", count.PageCount)
	}
	for i, want := range expected {
		page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}}
		size, e := inst.GetPageSize(&requests.GetPageSize{Page: page})
		if e != nil {
			t.Fatal(e)
		}
		if int(size.Width*2) != want.Width || int(size.Height*2) != want.Height {
			t.Fatal("page dimensions changed")
		}
		im, e := referenceRenderLCD(inst, doc.Document, page, want.Width, want.Height)
		if e != nil {
			t.Fatal(e)
		}
		refPage := requests.Page{ByIndex: &requests.PageByIndex{Document: ref.Document, Index: i}}
		wantImage, e := referenceRenderLCD(inst, ref.Document, refPage, want.Width, want.Height)
		if e != nil {
			t.Fatal(e)
		}
		if !sameImage(im, wantImage) {
			t.Errorf("%s page %d pixels differ from verified appearance", kind, i+1)
		}
	}
}
func TestReferenceRadioFillEverySelection(t *testing.T) {
	before := referenceFormSource(t)
	src := writeTemp(t, "source.pdf", before)
	for _, state := range []string{"A", "B", "C"} {
		t.Run(state, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "filled.pdf")
			if _, e := Fill(context.Background(), src, out, map[string]string{"answer": "/" + state}, false); e != nil {
				t.Fatal(e)
			}
			r, e := Report(context.Background(), out)
			if e != nil {
				t.Fatal(e)
			}
			if r.Fields["answer"] == nil || *r.Fields["answer"] != "/"+state || len(r.Issues) != 0 {
				t.Fatal(r)
			}
			if len(r.Widgets) < 3 {
				t.Fatalf("radio widget count = %d", len(r.Widgets))
			}
			for i, widget := range r.Widgets[:3] {
				wantValue := "/Off"
				if []string{"A", "B", "C"}[i] == state {
					wantValue = "/" + state
				}
				if widget.Value == nil || *widget.Value != wantValue {
					t.Errorf("widget %d value = %v, want %s", i, widget.Value, wantValue)
				}
			}
			b, e := os.ReadFile(out)
			if e != nil {
				t.Fatal(e)
			}
			want := []bool{state == "A", state == "B", state == "C"}
			if got := radioMarks(referenceFormImages(t, b)[0]); !equalBools(got, want) {
				t.Fatal(got, want)
			}
			referenceFormUnchanged(t, src, before)
		})
	}
}
func TestReferenceRadioRejectsSavedAndVisibleState(t *testing.T) {
	for key, value := range map[string]string{"AS": "Off", "V": "Unknown"} {
		t.Run(key, func(t *testing.T) {
			data := referenceFormMutation(t, func(c *model.Context) { referenceWidgets(t, c, 1)[0][key] = types.Name(value) })
			src := writeTemp(t, "invalid.pdf", data)
			r, e := Report(context.Background(), src)
			if e != nil {
				t.Fatal(e)
			}
			if len(r.Issues) == 0 {
				t.Fatal("missing validation issue")
			}
			out := filepath.Join(t.TempDir(), "output.pdf")
			if _, e = Fill(context.Background(), src, out, nil, false); e == nil || !strings.Contains(e.Error(), "validation") {
				t.Fatalf("expected validation error, got %v", e)
			}
			if _, e = os.Stat(out); !os.IsNotExist(e) {
				t.Fatal("output published", e)
			}
			referenceFormUnchanged(t, src, data)
		})
	}
}
func TestReferenceFlattenFilledAllPages(t *testing.T) {
	before := referenceFormSource(t)
	src := writeTemp(t, "source.pdf", before)
	out := filepath.Join(t.TempDir(), "flat.pdf")
	if _, e := Fill(context.Background(), src, out, map[string]string{"answer": "/B", "name": "Ada"}, true); e != nil {
		t.Fatal(e)
	}
	referenceNoForm(t, out)
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	if !equalBools(radioMarks(referenceFormImages(t, b)[0]), []bool{false, true, false}) {
		t.Error("wrong radio marks")
	}
	texts, e := PageText(context.Background(), b)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []string{"Ada", "Alpha"} {
		if !strings.Contains(texts[1], v) {
			t.Error("missing text", v)
		}
	}
	referenceFormPixels(t, out, "filled")
	referenceFormUnchanged(t, src, before)
}
func referenceNoForm(t *testing.T, path string) {
	t.Helper()
	r, e := Report(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	if r.HasFormTree || len(r.Widgets) != 0 {
		t.Fatal("form tree or widgets remain", r)
	}
}
func TestReferenceFlattenExistingResource(t *testing.T) {
	var appearance []byte
	before := referenceFormMutation(t, func(c *model.Context) {
		w := referenceWidgets(t, c, 1)[0]
		normal := dictOf(c.XRefTable, dictOf(c.XRefTable, w["AP"]).d["N"]).d
		sd, _, e := c.DereferenceStreamDict(normal["A"])
		if e != nil {
			t.Fatal(e)
		}
		if e = sd.Decode(); e != nil {
			t.Fatal(e)
		}
		appearance = append([]byte{}, sd.Content...)
		page, _, _, e := c.PageDict(context.Background(), 1, false)
		if e != nil {
			t.Fatal(e)
		}
		dictOf(c.XRefTable, page["Resources"]).d["XObject"] = types.Dict{"Fm_widget_0": normal["A"]}
	})
	src := writeTemp(t, "source.pdf", before)
	out := filepath.Join(t.TempDir(), "flat.pdf")
	if _, e := Fill(context.Background(), src, out, nil, true); e != nil {
		t.Fatal(e)
	}
	referenceNoForm(t, out)
	c, _, e := LoadFile(context.Background(), out)
	if e != nil {
		t.Fatal(e)
	}
	page, _, _, e := c.PageDict(context.Background(), 1, false)
	if e != nil {
		t.Fatal(e)
	}
	xs := dictOf(c.XRefTable, dictOf(c.XRefTable, page["Resources"]).d["XObject"]).d
	sd, _, e := c.DereferenceStreamDict(xs["Fm_widget_0"])
	if e != nil || sd == nil {
		t.Fatal("lost XObject", e)
	}
	if e = sd.Decode(); e != nil {
		t.Fatal(e)
	}
	if string(sd.Content) != string(appearance) {
		t.Error("existing appearance changed")
	}
	referenceFormPixels(t, out, "resource")
	referenceFormUnchanged(t, src, before)
}
func TestReferenceFlattenStaleAndMissingAppearance(t *testing.T) {
	for _, kind := range []string{"stale", "missing"} {
		t.Run(kind, func(t *testing.T) {
			before := referenceFormMutation(t, func(c *model.Context) {
				w := referenceWidgets(t, c, 2)[0]
				w["V"] = types.StringLiteral("Current account")
				if kind == "missing" {
					delete(w, "AP")
				}
			})
			src := writeTemp(t, "source.pdf", before)
			out := filepath.Join(t.TempDir(), "flat.pdf")
			if _, e := Fill(context.Background(), src, out, nil, true); e != nil {
				t.Fatal(e)
			}
			referenceNoForm(t, out)
			b, e := os.ReadFile(out)
			if e != nil {
				t.Fatal(e)
			}
			text, e := PageText(context.Background(), b)
			if e != nil {
				t.Fatal(e)
			}
			if !strings.Contains(text[1], "Current account") || strings.Contains(text[1], "Original") {
				t.Error(text[1])
			}
			referenceNoForm(t, out)
			referenceFormPixels(t, out, kind)
			referenceFormUnchanged(t, src, before)
		})
	}
}
func TestReferenceFlattenTransformedAppearances(t *testing.T) {
	for i, m := range [][]float64{{1, 0, 0, 1, 0, 0}, {0, 1, -1, 0, 20, 0}, {.5, 0, 0, .5, 8, 12}, {1, 0, 0, 1, 0, 0}} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			before := referenceFormMutation(t, func(c *model.Context) {
				w := referenceWidgets(t, c, 1)[3]
				w["Rect"] = types.NewNumberArray(150, 280, 210, 330)
				normal := dictOf(c.XRefTable, dictOf(c.XRefTable, w["AP"]).d["N"]).d
				for _, o := range normal {
					sd, _, e := c.DereferenceStreamDict(o)
					if e != nil {
						t.Fatal(e)
					}
					a := types.Array{}
					for _, v := range m {
						a = append(a, types.Float(v))
					}
					sd.Dict["Matrix"] = a
					sd.Dict["BBox"] = types.NewNumberArray(0, 0, 20, 20)
					if i == 3 {
						sd.Dict["BBox"] = types.NewNumberArray(5, 3, 25, 23)
					}
				}
			})
			src := writeTemp(t, "source.pdf", before)
			out := filepath.Join(t.TempDir(), "flat.pdf")
			if _, e := Fill(context.Background(), src, out, nil, true); e != nil {
				t.Fatal(e)
			}
			referenceFormPixels(t, out, fmt.Sprint("transform-", i))
			referenceFormUnchanged(t, src, before)
		})
	}
}
func TestReferenceFlattenInvalidGeometryNoOutput(t *testing.T) {
	before := referenceFormMutation(t, func(c *model.Context) {
		w := referenceWidgets(t, c, 1)[3]
		normal := dictOf(c.XRefTable, dictOf(c.XRefTable, w["AP"]).d["N"]).d
		sd, _, e := c.DereferenceStreamDict(normal["Yes"])
		if e != nil {
			t.Fatal(e)
		}
		sd.Dict["BBox"] = types.NewNumberArray(0, 0, 0, 20)
	})
	src := writeTemp(t, "source.pdf", before)
	out := filepath.Join(t.TempDir(), "invalid.pdf")
	if _, e := Fill(context.Background(), src, out, nil, true); e == nil || !strings.Contains(e.Error(), "geometry") {
		t.Fatal("expected geometry error", e)
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("output published", e)
	}
	referenceFormUnchanged(t, src, before)
}
func TestReferenceFlattenPageGraphicsState(t *testing.T) {
	before := referenceFormMutation(t, func(c *model.Context) {
		p, _, _, e := c.PageDict(context.Background(), 1, false)
		if e != nil {
			t.Fatal(e)
		}
		ref, e := addStream(c.XRefTable, []byte("2 0 0 2 10 10 cm\n0 0 10 10 re W n"))
		if e != nil {
			t.Fatal(e)
		}
		p["Contents"] = types.Array{p["Contents"], ref}
	})
	src := writeTemp(t, "source.pdf", before)
	out := filepath.Join(t.TempDir(), "flat.pdf")
	if _, e := Fill(context.Background(), src, out, nil, true); e != nil {
		t.Fatal(e)
	}
	referenceFormPixels(t, out, "state")
	referenceFormUnchanged(t, src, before)
}

func referenceFormImages(t *testing.T, data []byte) []*image.RGBA {
	t.Helper()
	inst, done, err := engine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		t.Fatal(err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		t.Fatal(err)
	}
	var out []*image.RGBA
	for i := 0; i < count.PageCount; i++ {
		page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}}
		size, err := inst.GetPageSize(&requests.GetPageSize{Page: page})
		if err != nil {
			t.Fatal(err)
		}
		img, err := referenceRenderFormPage(inst, doc.Document, page, int(math.Ceil(size.Width*2)), int(math.Ceil(size.Height*2)), enums.FPDF_RENDER_FLAG_ANNOT)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, img)
	}
	return out
}
