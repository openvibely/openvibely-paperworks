package cli

import "golang.org/x/sys/unix"

func renamePreviewExclusive(stage, out string) error {
	return renamePreviewExclusiveAtBoundary(stage, out, nil)
}

func renamePreviewExclusiveNative(stage, out string) error {
	return unix.RenamexNp(stage, out, unix.RENAME_EXCL)
}
