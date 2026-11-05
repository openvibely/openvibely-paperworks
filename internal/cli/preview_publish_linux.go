package cli

import "golang.org/x/sys/unix"

func renamePreviewExclusive(stage, out string) error {
	return renamePreviewExclusiveAtBoundary(stage, out, nil)
}

func renamePreviewExclusiveNative(stage, out string) error {
	return unix.Renameat2(unix.AT_FDCWD, stage, unix.AT_FDCWD, out, unix.RENAME_NOREPLACE)
}
