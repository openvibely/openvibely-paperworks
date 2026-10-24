package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// An atomic directory creation coordinates separate CLI processes. Fail fast:
// callers may retry after the active publisher finishes. Never steal a lock;
// an interrupted publisher's lock requires explicit cleanup.
func lockSlidePreview(out string) (func(), error) {
	absolute, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	lock := filepath.Join(parent, "."+filepath.Base(absolute)+".paperworks-preview-lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("preview publication is locked: %s; retry after the other publisher finishes (if it was interrupted, remove this lock only after confirming no publisher is running)", lock)
		}
		return nil, err
	}
	return func() { _ = os.Remove(lock) }, nil
}
