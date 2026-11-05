package convert

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// NewOutputDir creates a fresh output directory, refusing existing paths so
// stale results cannot be mistaken for fresh ones.
func NewOutputDir(path string) error {
	_, err := createOutputDir(path)
	return err
}

// createOutputDir creates the final path atomically and returns its identity so
// a failed caller can remove only the directory it created.
func createOutputDir(path string) (os.FileInfo, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("output directory already exists: %s: %w", path, os.ErrExist)
		}
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func removeOwnedOutputDir(path string, identity os.FileInfo) error {
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(identity, current) {
		return nil
	}
	return os.RemoveAll(path)
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
