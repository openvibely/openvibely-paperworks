package cli

import (
	"context"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func init() {
	register(&Command{Name: "extended-charts", Summary: "List, create, or replace literal data in editable treemap, sunburst and histogram charts.", Usage: "paperworks extended-charts INPUT.pptx [--requirements CHART.json --output NEW.pptx]", Run: func(ctx context.Context, a Args) (any, error) {
		file, r, err := listOrEdit(ctx, a, "extended-charts", "PPTX", office.ExtendedChartSpec{},
			func(s office.ExtendedChartSpec) string { return s.Action }, office.ExtendedCharts)
		if err != nil {
			return nil, err
		}
		return map[string]any{"file": file, "charts": r}, nil
	}})
}
