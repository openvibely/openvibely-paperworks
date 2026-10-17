package testsupport

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The unchanged Reference mutate helper follows destination symlinks, including a
// link back to its source. Source preservation is not guaranteed for aliases.
func TestMutationDestinationSymlinks(t *testing.T) {
	for _, kind := range []string{"existing", "dangling", "source-alias", "loop", "callback-error"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			outputRoot := filepath.Join(root, "outputs")
			if err := os.Mkdir(outputRoot, 0700); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "source.zip")
			if err := SaveStoredZIP(source, &OrderedZIP{Parts: []ZIPPart{{"original", "keep"}}}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, "target.zip")
			if kind == "existing" || kind == "callback-error" {
				if err := os.WriteFile(target, []byte("sentinel"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "source-alias" {
				target = source
			}
			link := filepath.Join(outputRoot, "1.pptx")
			linkTarget := target
			if kind == "loop" {
				linkTarget = link
			}
			if err := os.Symlink(linkTarget, link); err != nil {
				t.Fatal(err)
			}
			z, err := zip.OpenReader(source)
			if err != nil {
				t.Fatal(err)
			}
			parts, err := ReadOrderedZIP(z)
			closeErr := z.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
			calls := 0
			callbackError := errors.New("callback sentinel")
			callback := func(p *OrderedZIP) error {
				calls++
				if len(p.Parts) != 1 || p.Parts[0] != (ZIPPart{"original", "keep"}) {
					t.Fatal("callback input", p.Parts)
				}
				p.Set("added", []byte("value"))
				if kind == "callback-error" {
					return callbackError
				}
				return nil
			}
			err = callback(parts)
			if err == nil {
				dst := PresentationOutput(t, outputRoot)
				if dst != link {
					t.Fatalf("allocated %s, want %s", dst, link)
				}
				err = SaveStoredZIP(dst, parts)
			}
			switch kind {
			case "loop":
				if !errors.Is(err, syscall.ELOOP) {
					t.Fatalf("loop error: %v", err)
				}
			case "callback-error":
				if !errors.Is(err, callbackError) {
					t.Fatal(err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			if calls != 1 {
				t.Fatal("callback count", calls)
			}
			st, err := os.Lstat(link)
			if err != nil || st.Mode()&os.ModeSymlink == 0 {
				t.Fatal("symlink replaced", err)
			}
			entries, err := os.ReadDir(outputRoot)
			if err != nil || len(entries) != 1 || entries[0].Name() != "1.pptx" {
				t.Fatal("output entries", entries, err)
			}
			after, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(before, after) != (kind != "source-alias") {
				t.Fatal("unexpected source modification")
			}
			if kind == "callback-error" {
				raw, err := os.ReadFile(target)
				if err != nil || string(raw) != "sentinel" {
					t.Fatal("target changed", err)
				}
			}
			if kind == "loop" || kind == "callback-error" {
				return
			}
			saved, err := zip.OpenReader(link)
			if err != nil {
				t.Fatal(err)
			}
			defer saved.Close()
			if len(saved.File) != 2 {
				t.Fatal("member count", len(saved.File))
			}
			for i, want := range []ZIPPart{{"original", "keep"}, {"added", "value"}} {
				f := saved.File[i]
				r, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, readErr := io.ReadAll(r)
				closeErr := r.Close()
				if readErr != nil || closeErr != nil || f.Name != want.Name || string(data) != want.Data || f.Method != zip.Store {
					t.Fatalf("member %d: %s %q %v %v", i, f.Name, data, readErr, closeErr)
				}
			}
		})
	}
}
