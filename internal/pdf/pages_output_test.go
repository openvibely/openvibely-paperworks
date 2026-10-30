package pdf

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNewCloseFailureRemovesPartialOutput(t *testing.T) {
	data := []byte("complete output")
	closeErr := errors.New("injected close failure")
	writers := []struct {
		name  string
		write func(io.Writer) error
	}{
		{
			name: "byte buffer",
			write: func(w io.Writer) error {
				_, err := w.Write(data)
				return err
			},
		},
		{
			name: "callback stream",
			write: func(w io.Writer) error {
				_, err := io.Copy(w, bytes.NewReader(data))
				return err
			},
		},
	}

	for _, tc := range writers {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output.pdf")
			err := writeNewWithCloser(path, tc.write, func(f *os.File) error {
				if err := f.Close(); err != nil {
					return err
				}
				return closeErr
			})
			if !errors.Is(err, closeErr) {
				t.Fatalf("err=%v, want injected close error", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("partial output remains after close failure: stat err=%v", err)
			}
		})
	}
}

func TestWriteNewOutputSuccessPreservesCompleteOutput(t *testing.T) {
	data := []byte("complete output")
	tests := []struct {
		name  string
		write func(string) error
	}{
		{
			name: "byte buffer",
			write: func(path string) error {
				return writeNew(path, data)
			},
		},
		{
			name: "callback stream",
			write: func(path string) error {
				return writeNewWith(path, func(w io.Writer) error {
					_, err := io.Copy(w, bytes.NewReader(data))
					return err
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output.pdf")
			if err := tc.write(path); err != nil {
				t.Fatalf("write output: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("output=%q, want %q", got, data)
			}
		})
	}
}

func TestWriteNewOutputLeavesExistingDestinationUnchanged(t *testing.T) {
	original := []byte("existing output")
	data := []byte("replacement output")
	tests := []struct {
		name  string
		write func(string) error
	}{
		{
			name: "byte buffer",
			write: func(path string) error {
				return writeNew(path, data)
			},
		},
		{
			name: "callback stream",
			write: func(path string) error {
				return writeNewWith(path, func(w io.Writer) error {
					_, err := io.Copy(w, bytes.NewReader(data))
					return err
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output.pdf")
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := tc.write(path); err == nil {
				t.Fatal("expected exclusive-create error")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read existing destination: %v", err)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("existing destination=%q, want %q", got, original)
			}
		})
	}
}
