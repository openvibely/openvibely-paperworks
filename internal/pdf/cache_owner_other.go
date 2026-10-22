//go:build !unix

package pdf

import (
	"io/fs"
	"os"
)

var cacheUID = os.Getuid

// Ownership cannot be verified here, so the persistent cache stays disabled.
func fileOwner(fs.FileInfo) (int, bool) { return 0, false }
