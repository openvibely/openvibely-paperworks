//go:build !darwin && !linux && !windows

package cli

import "errors"

// On platforms without an atomic no-replace rename primitive, refusing to
// publish is safer than checking the destination and then possibly replacing
// a directory created by another process.
func renamePreviewExclusive(stage, out string) error {
	return errors.New("atomic no-replace preview publication is unsupported on this platform")
}
