package cli

import (
	"context"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

// listOrEdit runs the shared read-only list / new-file edit contract. Specs whose
// action is empty or "list" are read-only and must not name --output.
func listOrEdit[S, R any](ctx context.Context, a Args, name, format string, spec S, action func(S) string, run func(*opc.Package, S) ([]R, error)) (string, []R, error) {
	in, err := input(a, 1)
	if err != nil {
		return "", nil, err
	}
	if ext(in[0]) != "."+strings.ToLower(format) {
		return "", nil, usagef("%s requires %s", name, format)
	}
	if a.Str("requirements") != "" {
		if err = a.JSON("requirements", &spec); err != nil {
			return "", nil, err
		}
	}
	act := action(spec)
	edit := act != "" && act != "list"
	out := a.Path("output")
	if edit {
		if err = newFile(out, in[0]); err != nil {
			return "", nil, err
		}
	} else if out != "" {
		return "", nil, usagef("list is read-only; omit --output")
	}
	p, err := opc.Open(in[0])
	if err != nil {
		return "", nil, err
	}
	r, err := run(p, spec)
	if err != nil {
		return "", nil, err
	}
	if err = ctx.Err(); err != nil {
		return "", nil, err
	}
	if r == nil {
		r = []R{}
	}
	if !edit {
		return in[0], r, nil
	}
	if err = p.SaveNew(out); err != nil {
		return "", nil, err
	}
	return out, r, nil
}
