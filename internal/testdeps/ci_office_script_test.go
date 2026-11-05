package testdeps

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVerifyOfficeDepsChecksPackagesAndPrintsVersions(t *testing.T) {
	packages := sharedOfficePackages(t)
	bin, logPath := fakeOfficeDependencyCommands(t)
	command := exec.Command("/bin/bash", officeDependencyScript())
	command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "DPKG_QUERY_LOG="+logPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("verification failed: %v\n%s", err, output)
	}

	for _, packageName := range packages {
		if !strings.Contains(string(output), packageName+"=") {
			t.Errorf("verification output does not contain package %q:\n%s", packageName, output)
		}
	}
	for _, expected := range []string{"LibreOffice 25.2", "v22.0.0"} {
		if !strings.Contains(string(output), expected) {
			t.Errorf("verification output does not contain %q:\n%s", expected, output)
		}
	}

	queriedPackages, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, packageName := range packages {
		if !strings.Contains(string(queriedPackages), packageName+"\n") {
			t.Errorf("dpkg-query was not asked to verify %s: %s", packageName, queriedPackages)
		}
	}
}

func TestVerifyOfficeDepsFailsWhenPackageIsMissing(t *testing.T) {
	packages := sharedOfficePackages(t)
	missingPackage := packages[len(packages)-1]
	bin, logPath := fakeOfficeDependencyCommands(t)
	command := exec.Command("/bin/bash", officeDependencyScript())
	command.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DPKG_QUERY_LOG="+logPath,
		"MISSING_PACKAGE="+missingPackage,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("verification passed with a missing package:\n%s", output)
	}
	if !strings.Contains(string(output), "Required office test package is not installed: "+missingPackage) {
		t.Fatalf("missing package was not identified:\n%s", output)
	}
}

func TestOfficeInstallWorkflowExpandsSharedPackageList(t *testing.T) {
	packages := sharedOfficePackages(t)
	workflowScript := officeInstallWorkflowScript(t)
	if !strings.Contains(workflowScript, "source scripts/office-test-packages.sh") {
		t.Fatal("workflow install step does not source the shared package list")
	}
	if !strings.Contains(workflowScript, `"${office_test_packages[@]}"`) {
		t.Fatal("workflow install step does not expand the shared package list")
	}

	bin, dpkgLogPath := fakeOfficeDependencyCommands(t)
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	runnerTemp := filepath.Join(tmp, "runner-temp")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runnerTemp, 0755); err != nil {
		t.Fatal(err)
	}
	startTimePath := filepath.Join(runnerTemp, "office-setup-start-ns")
	startTime := strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(startTimePath, []byte(startTime), 0600); err != nil {
		t.Fatal(err)
	}

	sudoLogPath := filepath.Join(tmp, "sudo.log")
	sudoPath := filepath.Join(bin, "sudo")
	if err := os.WriteFile(sudoPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SUDO_LOG\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	summaryPath := filepath.Join(tmp, "step-summary.md")
	command := exec.Command("/bin/bash", "-e", "-o", "pipefail", "-c", workflowScript)
	command.Dir = officeRepositoryRoot()
	command.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DPKG_QUERY_LOG="+dpkgLogPath,
		"SUDO_LOG="+sudoLogPath,
		"HOME="+home,
		"RUNNER_TEMP="+runnerTemp,
		"GITHUB_STEP_SUMMARY="+summaryPath,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("workflow install step failed: %v\n%s", err, output)
	}

	sudoLog, err := os.ReadFile(sudoLogPath)
	if err != nil {
		t.Fatal(err)
	}
	aptArchives := filepath.Join(home, ".cache", "paperworks-apt", "archives") + "/"
	sudoInvocations := strings.Split(strings.TrimSpace(string(sudoLog)), "\n")
	for _, expected := range []string{
		"apt-get -o Dir::Cache::archives=" + aptArchives + " update",
		"apt-get -o Dir::Cache::archives=" + aptArchives + " autoclean",
	} {
		if !strings.Contains(string(sudoLog), expected+"\n") {
			t.Errorf("workflow omitted apt invocation %q:\n%s", expected, sudoLog)
		}
	}

	installInvocation := ""
	for _, invocation := range sudoInvocations {
		if strings.Contains(invocation, " install -y --no-install-recommends ") {
			installInvocation = invocation
			break
		}
	}
	if installInvocation == "" {
		t.Fatalf("workflow did not run the apt install command:\n%s", sudoLog)
	}

	wantInvocation := strings.Join(append([]string{
		"apt-get",
		"-o", "Dir::Cache::archives=" + aptArchives,
		"-o", "APT::Keep-Downloaded-Packages=true",
		"install", "-y", "--no-install-recommends",
	}, packages...), " ")
	if installInvocation != wantInvocation {
		t.Fatalf("workflow apt install arguments differ from the shared package list:\n got: %s\nwant: %s", installInvocation, wantInvocation)
	}
}

func sharedOfficePackages(t *testing.T) []string {
	t.Helper()
	command := exec.Command("/bin/bash", "-c", `source "$1"; printf '%s\n' "${office_test_packages[@]}"`, "bash", officePackageList())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("reading shared office package list failed: %v\n%s", err, output)
	}
	var packages []string
	for _, packageName := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if packageName != "" {
			packages = append(packages, packageName)
		}
	}
	if len(packages) == 0 {
		t.Fatal("shared office package list is empty")
	}
	return packages
}

func officeInstallWorkflowScript(t *testing.T) string {
	t.Helper()
	workflow, err := os.ReadFile(filepath.Join(officeRepositoryRoot(), ".github", "workflows", "test.yml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(workflow), "\n")
	foundStep := false
	foundRun := false
	var script []string
	for _, line := range lines {
		if !foundStep {
			if strings.TrimSpace(line) == "- name: Install and verify Office test dependencies" {
				foundStep = true
			}
			continue
		}
		if !foundRun {
			if strings.TrimSpace(line) == "run: |" {
				foundRun = true
			}
			continue
		}
		if strings.TrimSpace(line) != "" && len(line)-len(strings.TrimLeft(line, " ")) < 10 {
			break
		}
		if strings.TrimSpace(line) == "" {
			script = append(script, "")
			continue
		}
		script = append(script, strings.TrimPrefix(line, "          "))
	}
	if !foundStep || !foundRun || len(script) == 0 {
		t.Fatal("could not extract workflow install step")
	}
	return strings.Join(script, "\n")
}

func officeDependencyScript() string {
	return filepath.Join(officeRepositoryRoot(), "scripts", "verify-office-deps.sh")
}

func officePackageList() string {
	return filepath.Join(officeRepositoryRoot(), "scripts", "office-test-packages.sh")
}

func officeRepositoryRoot() string {
	_, currentFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(currentFile), "..", "..")
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
		"soffice": "#!/bin/sh\nprintf 'LibreOffice 25.2\\n'\n",
		"node":    "#!/bin/sh\nprintf 'v22.0.0\\n'\n",
	}
	for name, body := range commands {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte(body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return bin, logPath
}
