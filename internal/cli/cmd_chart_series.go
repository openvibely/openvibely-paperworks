package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

type ChartSeriesSpec struct {
	Part       string    `json:"part"`
	Name       string    `json:"name"`
	Categories []string  `json:"categories"`
	Values     []float64 `json:"values"`
}

func init() {
	register(&Command{Name: "chart-series-replace", Summary: "Replace a single-series column chart's categories and embedded workbook; preserves the chart's placement and formatting.", Usage: "paperworks chart-series-replace INPUT --spec DATA.json --output NEW_FILE", Run: func(ctx context.Context, a Args) (any, error) {
		in, out, err := oneInOut(a)
		if err != nil {
			return nil, err
		}
		var s ChartSeriesSpec
		if err = a.JSON("spec", &s); err != nil {
			return nil, err
		}
		p, err := opc.Open(in)
		if err != nil {
			return nil, err
		}
		if err = office.ReplaceCategorySeries(p, s.Part, s.Name, s.Categories, s.Values); err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		err = p.SaveNew(out)
		return map[string]any{"output": out, "embedded_workbook_replaced": true, "visual_review_required": true}, err
	}})
}
