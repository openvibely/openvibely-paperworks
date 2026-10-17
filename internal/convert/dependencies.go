package convert

import (
	"context"

	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
)

// Per-call dependencies let native tests exercise converter failures and routing
// without replacing process-wide state or exposing test switches in the CLI.
type dependencies struct {
	refresh func(context.Context, string, string, bool) error
	export  func(context.Context, string, string, string) (string, error)
	convert func(context.Context, string, string, Options) (map[string]any, error)
	render  func(context.Context, string, string, pdf.RenderOptions) ([]string, error)
}
type dependenciesKey struct{}

func deps(ctx context.Context) dependencies {
	d, _ := ctx.Value(dependenciesKey{}).(dependencies)
	if d.refresh == nil {
		d.refresh = lo.Refresh
	}
	if d.export == nil {
		d.export = lo.Convert
	}
	if d.convert == nil {
		d.convert = Convert
	}
	if d.render == nil {
		d.render = pdf.RenderPages
	}
	return d
}
