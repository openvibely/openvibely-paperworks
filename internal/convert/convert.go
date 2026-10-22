// Package convert drives LibreOffice conversions and PDFium rendering of
// disposable copies; delivered documents are never modified.
package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

type Options struct {
	Recalculate bool
	Refresh     bool
	Sheet       string
	CellRange   string
	WholeSheets bool
}

const Engine = "LibreOffice; compare critical formulas with the target spreadsheet application"

func ext(p string) string { return strings.ToLower(filepath.Ext(p)) }

func stem(p string) string { return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)) }

func externalResources(report office.Report) bool {
	links, _ := report["external_relationships"].([]office.ExtRel)
	for _, l := range links {
		if !strings.HasSuffix(l.Type, "/hyperlink") {
			return true
		}
	}
	return false
}

// Convert exports a DOCX/XLSX/PPTX to PDF (default), a recalculated XLSX, or a
// refreshed copy in the same format, inside a new output directory.
func Convert(ctx context.Context, path, output string, o Options) (map[string]any, error) {
	e := ext(path)
	if e != ".docx" && e != ".xlsx" && e != ".pptx" {
		return nil, errors.New("convert an explicit DOCX/XLSX/PPTX copy; other formats need a separate workflow")
	}
	if (o.Sheet != "" || o.CellRange != "") && (e != ".xlsx" || o.Sheet == "" || o.Recalculate || o.Refresh) {
		return nil, errors.New("preview selection requires XLSX rendering and --sheet")
	}
	if o.WholeSheets && (e != ".xlsx" || o.Recalculate || o.Refresh || o.CellRange != "") {
		return nil, errors.New("whole-sheet sizing requires XLSX preview without a range")
	}
	report, err := office.PackageReport(path)
	if err != nil {
		return nil, err
	}
	if externalResources(report) {
		return nil, errors.New("External resources require review/removal on a copy before conversion")
	}
	if err := NewOutputDir(output); err != nil {
		return nil, err
	}
	target := "pdf"
	if o.Refresh {
		target = strings.TrimPrefix(e, ".")
	} else if o.Recalculate {
		target = "xlsx"
	}
	result := filepath.Join(output, stem(path)+"."+target)
	temp, err := os.MkdirTemp("", "paperworks-office-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	source := path
	if e == ".xlsx" && target == "pdf" {
		dir := filepath.Join(temp, "selection")
		os.Mkdir(dir, 0o755)
		source = filepath.Join(dir, filepath.Base(path))
		if err := office.WorkbookPreviewCopy(path, source, o.Sheet, o.CellRange); err != nil {
			return nil, err
		}
	}
	if target == "pdf" {
		need, err := office.NeedsOfficePreview(source)
		if err != nil {
			return nil, err
		}
		if need {
			if e == ".xlsx" {
				formulas, _ := report["formulas"].([]office.Formula)
				if len(formulas) > 0 {
					calc, err := Convert(ctx, source, filepath.Join(temp, "calculated"), Options{Recalculate: true})
					if err != nil {
						return nil, err
					}
					source = calc["output"].(string)
				}
			}
			dir := filepath.Join(temp, "preview")
			os.Mkdir(dir, 0o755)
			preview := filepath.Join(dir, filepath.Base(path))
			changed, err := office.OfficePreviewCopy(source, preview)
			if err != nil {
				return nil, err
			}
			// A saved preview can retain native chart parts while its slide
			// already contains fallback drawings. No change means no file was
			// written; keep exporting the existing source in that case.
			if changed {
				source = preview
			}
		}
	}
	// Plain export may trust stale cached formulas; spreadsheets are always recalculated.
	if e == ".xlsx" || o.Refresh {
		if err := deps(ctx).refresh(ctx, source, result, o.WholeSheets); err != nil {
			return nil, err
		}
		if o.WholeSheets {
			if err := SelectPreviewSheets(ctx, source, result); err != nil {
				return nil, err
			}
		}
	} else {
		staged := filepath.Join(temp, "out")
		os.Mkdir(staged, 0o755)
		out, err := deps(ctx).export(ctx, source, staged, target)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(out)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(result, data, 0o644); err != nil {
			return nil, err
		}
	}
	if st, err := os.Stat(result); err != nil || st.Size() == 0 {
		return nil, errors.New("office conversion produced no output")
	}
	if !o.Recalculate {
		return map[string]any{"output": result, "visually_inspected": false}, nil
	}
	final, err := checkRecalculated(path, result, report)
	if err != nil {
		os.Remove(result)
		return nil, err
	}
	return final, nil
}

func checkRecalculated(original, result string, before office.Report) (map[string]any, error) {
	after, err := office.PackageReport(result)
	if err != nil {
		return nil, err
	}
	if fmt.Sprint(before["sheets"]) != fmt.Sprint(after["sheets"]) {
		return nil, errors.New("Recalculation changed worksheet names or order")
	}
	formulas, _ := before["formulas"].([]office.Formula)
	have := map[[2]string]bool{}
	for _, f := range after["formulas"].([]office.Formula) {
		have[[2]string{f.Sheet, f.Cell}] = true
	}
	var missing []string
	for _, f := range formulas {
		if !have[[2]string{f.Sheet, f.Cell}] {
			missing = append(missing, f.Sheet+"!"+f.Cell)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, fmt.Errorf("Recalculation removed formulas: %v", missing[:min(20, len(missing))])
	}
	calculated, err := os.ReadFile(result)
	if err != nil {
		return nil, err
	}
	merged, err := office.PreserveFormulaCaches(original, calculated, formulas)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(result, merged, 0o644); err != nil {
		return nil, err
	}
	report, err := office.PackageReport(result)
	if err != nil {
		return nil, err
	}
	report["original_package_preserved_except_calculated_results"] = true
	report["formula_cells_preserved"] = true
	report["output"] = result
	report["engine"] = Engine
	return report, nil
}

// SelectPreviewSheets keeps visible sheets' pages from a disposable one-page-per-sheet
// export. The PDF is removed on failure so an invalid mapping cannot leak sheets.
// Never pass a user's original PDF: this operation rewrites or removes pdfPath.
func SelectPreviewSheets(ctx context.Context, workbook, pdfPath string) (err error) {
	defer func() {
		if err != nil {
			_ = os.Remove(pdfPath)
		}
	}()
	count, visible, err := office.VisibleSheetIndexes(workbook)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return err
	}
	pages, err := pdf.PageCount(ctx, data)
	if err != nil {
		return err
	}
	if pages != count {
		return errors.New("whole-sheet preview page count does not match workbook sheets")
	}
	if len(visible) == 0 {
		return errors.New("no visible worksheet to preview")
	}
	keep := make([]int, len(visible))
	for i, v := range visible {
		keep[i] = v + 1
	}
	out, err := pdf.SelectPreviewPages(ctx, data, keep)
	if err != nil {
		return err
	}
	return os.WriteFile(pdfPath, out, 0o644)
}
