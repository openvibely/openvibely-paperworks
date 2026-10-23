package convert

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// NewOutputDir creates a fresh output directory, refusing existing paths so
// stale results cannot be mistaken for fresh ones.
func NewOutputDir(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("output directory already exists: %s: %w", path, os.ErrExist)
	}
	return os.MkdirAll(path, 0o755)
}

// publishRenderPDF stages on the destination filesystem so replacement also
// works when conversion's temporary directory is on another volume.
func publishRenderPDF(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(destination), ".paperworks-pdf-*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Chmod(0o644); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(out.Name(), destination)
}
