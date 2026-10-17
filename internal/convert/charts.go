package convert

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// ChartInputSpec selects a chart (1-based part number or explicit part) and
// the literal workbook inputs to change.
type ChartInputSpec struct {
	Chart     int            `json:"chart"`
	ChartPart string         `json:"chart_part"`
	Sheet     string         `json:"sheet"`
	Cells     map[string]any `json:"cells"`
}

// WorkbookInputs patches literal cells in a new XLSX copy.
func WorkbookInputs(source, destination, sheet string, cells map[string]any) (map[string]any, error) {
	if _, err := os.Stat(destination); err == nil {
		return nil, errors.New("output exists: " + destination)
	}
	if ext(source) != ".xlsx" {
		return nil, errors.New("input edits require XLSX")
	}
	if _, err := office.PackageReport(source); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	out, part, err := office.WorkbookInputs(raw, sheet, cells)
	if err != nil {
		return nil, err
	}
	if err := opc.WriteNew(destination, out); err != nil {
		return nil, err
	}
	return map[string]any{"output": destination, "changed_parts": []string{part},
		"next": "Recalculate the new copy before relying on dependent results; prior caches are retained for explicit snapshot workflows."}, nil
}

// ChartInputs edits literal inputs of a chart's embedded workbook,
// recalculates it with LibreOffice and refreshes every chart cache bound to
// that workbook, without replacing chart design.
func ChartInputs(ctx context.Context, source, destination string, spec ChartInputSpec) (map[string]any, error) {
	if _, err := os.Stat(destination); err == nil {
		return nil, errors.New("output exists: " + destination)
	}
	if spec.Chart == 0 {
		spec.Chart = 1
	}
	if spec.Chart < 1 {
		return nil, errors.New("chart is a positive chart-part number")
	}
	if _, err := office.PackageReport(source); err != nil {
		return nil, err
	}
	p, err := opc.Open(source)
	if err != nil {
		return nil, err
	}
	names, err := office.ChartParts(p)
	if err != nil {
		return nil, err
	}
	name := spec.ChartPart
	if name == "" && spec.Chart <= len(names) {
		name = names[spec.Chart-1]
	}
	if !slices.Contains(names, name) {
		return nil, errors.New("unknown chart part")
	}
	target, err := chartTarget(p, name)
	if err != nil {
		return nil, err
	}
	if ext(target) != ".xlsx" {
		return nil, errors.New("editable chart input must be XLSX")
	}
	edited, _, err := office.WorkbookInputs(p.Get(target), spec.Sheet, spec.Cells)
	if err != nil {
		return nil, err
	}
	temp, err := os.MkdirTemp("", "document-chart-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	inputs := filepath.Join(temp, "inputs.xlsx")
	if err := os.WriteFile(inputs, edited, 0o600); err != nil {
		return nil, err
	}
	res, err := deps(ctx).convert(ctx, inputs, filepath.Join(temp, "calculated"), Options{Recalculate: true})
	if err != nil {
		return nil, err
	}
	calculated, err := os.ReadFile(res["output"].(string))
	if err != nil {
		return nil, err
	}
	nested, err := opc.Read(calculated)
	if err != nil {
		return nil, err
	}
	wb, err := office.OpenWorkbook(nested)
	if err != nil {
		return nil, err
	}
	p.Set(target, calculated)
	changed := []string{target}
	for _, n := range names {
		doc, err := p.XML(n)
		if err != nil {
			return nil, err
		}
		root := doc.Root()
		if office.Is(root, office.NSCX, "chartSpace") {
			if office.Child(office.Child(root, office.NSCX, "chartData"), office.NSCX, "externalData") == nil {
				continue
			}
		} else if office.Child(root, office.NSC, "externalData") == nil && len(office.All(root, office.NSC, "numRef"))+len(office.All(root, office.NSC, "strRef")) == 0 {
			continue
		}
		other, err := chartTarget(p, n)
		if err != nil {
			return nil, err
		}
		if other != target {
			continue
		}
		if office.Is(root, office.NSCX, "chartSpace") {
			_, err = office.ExtendedChartCache(root, wb, true)
		} else {
			err = office.RefreshChartReferences(root, wb)
		}
		if err != nil {
			return nil, err
		}
		if err := p.SetXML(n, doc); err != nil {
			return nil, err
		}
		changed = append(changed, n)
	}
	verified, err := office.VerifyChartData(p, "")
	if err != nil {
		return nil, err
	}
	if _, err := office.PackageReportOf(p); err != nil {
		return nil, err
	}
	if err := p.SaveNew(destination); err != nil {
		return nil, err
	}
	slices.Sort(changed)
	return map[string]any{"output": destination, "changed_parts": changed, "charts": verified,
		"next": "Update dependent slide titles, table values and notes explicitly, then render and review."}, nil
}

func chartTarget(p *opc.Package, name string) (string, error) {
	doc, err := p.XML(name)
	if err != nil {
		return "", err
	}
	return office.ChartWorkbookTarget(p, name, doc.Root())
}

// MaterializeCharts snapshots literal chart data into embedded workbooks.
func MaterializeCharts(source, destination string) (map[string]any, error) {
	if _, err := office.PackageReport(source); err != nil {
		return nil, err
	}
	if _, err := os.Stat(destination); err == nil {
		return nil, &os.PathError{Op: "create", Path: destination, Err: os.ErrExist}
	}
	p, err := opc.Open(source)
	if err != nil {
		return nil, err
	}
	made, err := office.MaterializeLiteralCharts(p)
	if err != nil {
		return nil, err
	}
	if _, err := office.VerifyChartData(p, ""); err != nil {
		return nil, err
	}
	if err := p.SaveNew(destination); err != nil {
		return nil, err
	}
	if made == nil {
		made = []string{}
	}
	return map[string]any{"output": destination, "materialized_charts": made,
		"lineage": "Literal values only; no original source formulas or links inferred."}, nil
}
