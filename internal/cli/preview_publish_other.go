//go:build !darwin && !linux

package cli

import "os"

func renamePreviewExclusive(stage, out string) error {
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		if err == nil {
			return os.ErrExist
		}
		return err
	}
	return os.Rename(stage, out)
}
