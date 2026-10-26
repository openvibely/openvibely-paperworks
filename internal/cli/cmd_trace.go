package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func init() {
	register(&Command{Name: "trace", ValueOpts: []string{"sheet", "cell", "max-nodes", "max-depth"}, Summary: "Trace bounded XLSX formula precedents without calculation or modification.", Usage: "paperworks trace INPUT.xlsx --sheet NAME --cell A1 [--max-nodes 200 --max-depth 20]", Run: func(ctx context.Context, a Args) (any, error) {
		in, e := input(a, 1)
		if e != nil {
			return nil, e
		}
		if e = a.Require("sheet", "cell"); e != nil {
			return nil, e
		}
		nodes, e := a.Int("max-nodes", 200)
		if e != nil {
			return nil, e
		}
		depth, e := a.Int("max-depth", 20)
		if e != nil {
			return nil, e
		}
		if nodes < 1 || nodes > 10000 || depth < 1 || depth > 100 {
			return nil, usagef("max-nodes must be 1–10000 and max-depth 1–100")
		}
		p, e := opc.Open(in[0])
		if e != nil {
			return nil, e
		}
		return office.TraceWorkbook(ctx, p, a.Str("sheet"), a.Str("cell"), nodes, depth)
	}})
}
