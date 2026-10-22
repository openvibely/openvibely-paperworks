// Package r1c1 contains shared helpers for interpreting R1C1 coordinates.
package r1c1

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	MaxRows    = 1048576 // Excel worksheet row limit.
	MaxColumns = 16384   // Excel worksheet column limit.
)

// ResolveComponent resolves one R1C1 row or column component against its
// origin. An empty component is relative to the origin, a bracketed component
// is a relative offset, and an unbracketed number is an absolute coordinate.
// The bool is true only for an unbracketed absolute coordinate.
func ResolveComponent(component string, origin, limit int) (coordinate int, absolute bool, err error) {
	if origin < 1 || origin > limit || limit < 1 {
		return 0, false, fmt.Errorf("R1C1 coordinate out of bounds")
	}
	if component == "" {
		return origin, false, nil
	}
	if strings.HasPrefix(component, "[") {
		if len(component) < 3 || component[len(component)-1] != ']' {
			return 0, false, fmt.Errorf("invalid R1C1 coordinate")
		}
		delta, parseErr := strconv.Atoi(component[1 : len(component)-1])
		if parseErr != nil {
			return 0, false, fmt.Errorf("invalid R1C1 coordinate")
		}
		if delta < 1-origin || delta > limit-origin {
			return 0, false, fmt.Errorf("R1C1 coordinate out of bounds")
		}
		return origin + delta, false, nil
	}
	value, parseErr := strconv.Atoi(component)
	if parseErr != nil {
		return 0, false, fmt.Errorf("invalid R1C1 coordinate")
	}
	if value < 1 || value > limit {
		return 0, false, fmt.Errorf("R1C1 coordinate out of bounds")
	}
	return value, true, nil
}
