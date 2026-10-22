//go:build unix

package pdf

import (
	"io/fs"
	"os"
	"syscall"
)

var cacheUID = os.Getuid

func fileOwner(st fs.FileInfo) (int, bool) {
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(sys.Uid), true
}
