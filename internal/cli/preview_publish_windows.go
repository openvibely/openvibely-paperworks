//go:build windows

package cli

import "os"

// os.Rename uses MoveFileW on Windows. MoveFileW atomically fails when the
// destination already exists, so this preserves exclusive publication without
// the replace behavior of MoveFileEx with MOVEFILE_REPLACE_EXISTING.
func renamePreviewExclusive(stage, out string) error {
	return os.Rename(stage, out)
}
