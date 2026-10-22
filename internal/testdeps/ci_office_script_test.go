package testdeps

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyOfficeDepsChecksPackagesAndPrintsVersions(t *testing.T) {
	bin, logPath := fakeOfficeDependencyCommands(t)
	command := exec.Command("/bin/bash", officeDependencyScript())
	command.Env = append(os.Environ(), "PATH="+bin, "DPKG_QUERY_LOG="+logPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("verification failed: %v\n%s", err, output)
	}

	for _, expected := range []string{
		"libreoffice-writer=",
		"libreoffice-calc=",
		"libreoffice-impress=",
		"fonts-liberation=",
		"fonts-dejavu-core=",
		"fonts-noto-core=",
		"LibreOffice 25.2",
		"v22.0.0",
	} {
		if !strings.Contains(string(output), expected) {
			t.Errorf("verification output does not contain %q:\n%s", expected, output)
		}
	}

	packages, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, packageName := range []string{
		"libreoffice-writer",
		"libreoffice-calc",
		"libreoffice-impress",
		"fonts-liberation",
		"fonts-dejavu-core",
		"fonts-noto-core",
	} {
		if !strings.Contains(string(packages), packageName+"\n") {
			t.Errorf("dpkg-query was not asked to verify %s: %s", packageName, packages)
		}
	}
}

func TestVerifyOfficeDepsFailsWhenPackageIsMissing(t *testing.T) {
	bin, logPath := fakeOfficeDependencyCommands(t)
	command := exec.Command("/bin/bash", officeDependencyScript())
	command.Env = append(os.Environ(),
		"PATH="+bin,
		"DPKG_QUERY_LOG="+logPath,
		"MISSING_PACKAGE=fonts-noto-core",
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("verification passed with a missing package:\n%s", output)
	}
	if !strings.Contains(string(output), "Required office test package is not installed: fonts-noto-core") {
		t.Fatalf("missing package was not identified:\n%s", output)
	}
}

func officeDependencyScript() string {
	_, currentFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(currentFile), "..", "..", "scripts", "verify-office-deps.sh")
}

func fakeOfficeDependencyCommands(t *testing.T) (string, string) {
	t.Helper()
	bin := t.TempDir()
	logPath := filepath.Join(bin, "dpkg-query.log")
	commands := map[string]string{
		"dpkg-query": `#!/bin/sh
case "$2" in
  *binary:Package*)
  shift 2
  for package in "$@"; do printf '%s\n' "$package" >> "$DPKG_QUERY_LOG"; printf '%s=1.0 (installed)\n' "$package"; done
  exit 0
  ;;
esac
printf '%s\n' "$3" >> "$DPKG_QUERY_LOG"
if [ "${MISSING_PACKAGE:-}" = "$3" ]; then exit 1; fi
printf 'installed'
`,
		"soffice": "#!/bin/sh\nprintf 'LibreOffice 25.2\n'\n",
		"node":    "#!/bin/sh\nprintf 'v22.0.0\n'\n",
	}
	for name, body := range commands {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte(body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return bin, logPath
}
