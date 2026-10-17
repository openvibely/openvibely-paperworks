// Package testdeps discovers native integration-test tools. It never downloads
// tools and is imported only by tests; make deps populates the shared cache.
package testdeps

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const sheetHash = "cc015130aa8521e7f088f88898eba949ccdcbfb38df0bd129b44b7273c3a6f41"

func cacheDir() (string, error) {
	// Match scripts/test-local.sh, including XDG overrides on macOS.
	if p := os.Getenv("XDG_CACHE_HOME"); p != "" {
		return filepath.Join(p, "paperworks-tests"), nil
	}
	p, err := os.UserCacheDir()
	return filepath.Join(p, "paperworks-tests"), err
}

func arch() string {
	if runtime.GOARCH == "amd64" {
		return "x64"
	}
	return runtime.GOARCH
}

func resolveOffice(explicit, cache string, find func() (string, error)) (string, error) {
	p, err := find()
	if err == nil {
		return p, nil
	}
	// Never mask an invalid explicit override with a cached installation.
	if explicit != "" {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		p = filepath.Join(cache, "libreoffice-25.8.7-"+arch(), "LibreOffice.app", "Contents", "MacOS", "soffice")
		if info, e := os.Stat(p); e == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w; run make deps from the repository root", err)
}

// Office preserves the production resolver's override/installed-tool precedence,
// then adds the test cache. The environment is inherited by CLI subprocesses.
func Office(find func() (string, error)) error {
	cache, err := cacheDir()
	if err != nil {
		return err
	}
	p, err := resolveOffice(os.Getenv("DOCUMENT_SOFFICE"), cache, find)
	if err != nil {
		return err
	}
	return os.Setenv("DOCUMENT_SOFFICE", p)
}

func reader(explicit, cache string) (string, error) {
	p := explicit
	if p == "" {
		p = filepath.Join(cache, "xlsx-0.20.3.full.min.js")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("SheetJS: %w; run make deps from the repository root", err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != sheetHash {
		return "", fmt.Errorf("SheetJS checksum mismatch: %s; run make deps or fix PAPERWORKS_TEST_SHEETJS", p)
	}
	return p, nil
}

func nodeOK(p string) bool {
	return exec.Command(p, "-e", `process.exit(Number(process.versions.node.split(".")[0]) >= 22 ? 0 : 1)`).Run() == nil
}

// SheetJS resolves the pinned reader and Node without requiring shell exports.
func SheetJS() error {
	cache, err := cacheDir()
	if err != nil {
		return err
	}
	p, err := reader(os.Getenv("PAPERWORKS_TEST_SHEETJS"), cache)
	if err != nil {
		return err
	}
	if err := os.Setenv("PAPERWORKS_TEST_SHEETJS", p); err != nil {
		return err
	}
	if node, err := exec.LookPath("node"); err == nil && nodeOK(node) {
		return nil
	}
	bin := filepath.Join(cache, "node-v22.22.2-"+runtime.GOOS+"-"+arch(), "bin")
	if !nodeOK(filepath.Join(bin, "node")) {
		return fmt.Errorf("Node 22+ unavailable; run make deps from the repository root")
	}
	return os.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
