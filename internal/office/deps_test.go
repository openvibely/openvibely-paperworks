package office

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/testdeps"
)

func officeTestFinder() (string, error) {
	if p := os.Getenv("DOCUMENT_SOFFICE"); p != "" {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
		return "", fmt.Errorf("DOCUMENT_SOFFICE=%s is not a file", p)
	}
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if root := os.Getenv(env); root != "" {
				candidates = append(candidates, filepath.Join(root, "LibreOffice", "program", "soffice.exe"))
			}
		}
	default:
		candidates = []string{"/usr/lib/libreoffice/program/soffice", "/opt/libreoffice/program/soffice", "/usr/bin/soffice"}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("LibreOffice not found")
}

func TestMain(m *testing.M) {
	if err := testdeps.Office(officeTestFinder); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := testdeps.SheetJS(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
