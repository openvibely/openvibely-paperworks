package convert

import (
	"fmt"
	"os"
)

// NewOutputDir creates a fresh output directory, refusing existing paths so
// stale results cannot be mistaken for fresh ones.
func NewOutputDir(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("output directory already exists: %s: %w", path, os.ErrExist)
	}
	return os.MkdirAll(path, 0o755)
}
