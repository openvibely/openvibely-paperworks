package convert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runExampleCheck preserves the original assert failure boundary between checks.
// A failed child must stop its owning phase before later checks or rendering.
func runExampleCheck(t *testing.T, name string, check func(*testing.T)) {
	t.Helper()
	if !t.Run(name, check) {
		t.FailNow()
	}
}

// installedExampleSourceRoot resolves the real source location before selecting
// its repository parent. This is the native test layout's counterpart to the
// original helper's __file__.resolve().parents[2] skill lookup. It does not
// emulate Reference loading or bootstrap behavior.
func installedExampleSourceRoot(sourceFile string) (string, error) {
	resolved, err := filepath.EvalSymlinks(sourceFile)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(resolved))), nil
}

// createExampleOutput mirrors mkdir(parents=True, exist_ok=False). In particular,
// an existing directory is an error even when empty, before any script executes.
func createExampleOutput(output string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	return os.Mkdir(output, 0755)
}

// The native examples are shell rather than Reference, but retain named script
// materialization, sequential shared-cwd execution, and per-script deadlines.
// Failure leaves the script and any partial outputs for inspection, as Reference does.
func runInstalledExample(ctx context.Context, shell, output, family string, index int, source string, env []string, timeout time.Duration) ([]byte, error) {
	script, err := filepath.Abs(filepath.Join(output, fmt.Sprintf("%s%d.sh", family, index)))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(script, []byte(source), 0644); err != nil {
		return nil, err
	}
	blockCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(blockCtx, shell, "-eu", script)
	cmd.Dir = output
	cmd.Env = env
	// CombinedOutput waits for pipe-copy goroutines after the direct process
	// exits. A surviving descendant can hold that pipe open past the deadline.
	// Reference run(..., timeout=...) inherits output and waits only for its direct
	// child. Use an unlinked capture file so descendants cannot delay Wait and
	// the capture is not visible in the shared example directory.
	capture, err := os.CreateTemp(output, ".example-capture-*")
	if err != nil {
		return nil, err
	}
	defer capture.Close()
	if err := os.Remove(capture.Name()); err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = capture, capture
	err = cmd.Run()
	info, statErr := capture.Stat()
	if statErr != nil {
		return nil, statErr
	}
	// Snapshot without moving the shared file offset or following a descendant
	// that continues writing after the direct child has exited.
	out, readErr := io.ReadAll(io.NewSectionReader(capture, 0, info.Size()))
	if readErr != nil {
		return out, readErr
	}
	if blockCtx.Err() != nil {
		return out, blockCtx.Err()
	}
	return out, err
}

func TestInstalledExampleExecutionContract(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("shared-directory-and-named-scripts", func(t *testing.T) {
		output := filepath.Join(t.TempDir(), "nested", "examples")
		if err := createExampleOutput(output); err != nil {
			t.Fatal(err)
		}
		sources := []string{"printf '%s\\n' \"$0\" > script-path\nprintf first > shared", "test \"$(cat shared)\" = first\nprintf second >> shared"}
		for i, source := range sources {
			if out, err := runInstalledExample(context.Background(), shell, output, "pdf", i, source, os.Environ(), 60*time.Second); err != nil {
				t.Fatalf("%s: %v", out, err)
			}
			data, err := os.ReadFile(filepath.Join(output, fmt.Sprintf("pdf%d.sh", i)))
			if err != nil || string(data) != source {
				t.Fatalf("script bytes: %q %v", data, err)
			}
		}
		data, err := os.ReadFile(filepath.Join(output, "script-path"))
		if err != nil || strings.TrimSpace(string(data)) != filepath.Join(output, "pdf0.sh") {
			t.Fatalf("script path %q %v", data, err)
		}
		data, err = os.ReadFile(filepath.Join(output, "shared"))
		if err != nil || string(data) != "firstsecond" {
			t.Fatalf("shared state %q %v", data, err)
		}
		if err := createExampleOutput(output); !errors.Is(err, os.ErrExist) {
			t.Fatalf("existing output: %v", err)
		}
		data, _ = os.ReadFile(filepath.Join(output, "shared"))
		if string(data) != "firstsecond" {
			t.Fatal("rejected output creation changed files")
		}
	})
	t.Run("process-failure-retains-partial-output", func(t *testing.T) {
		output := t.TempDir()
		source := "printf partial > partial\nprintf diagnostic >&2\nexit 7\nprintf unreachable > after"
		out, err := runInstalledExample(context.Background(), shell, output, "word", 0, source, os.Environ(), 60*time.Second)
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 7 || string(out) != "diagnostic" {
			t.Fatalf("failure %q %v", out, err)
		}
		data, e := os.ReadFile(filepath.Join(output, "partial"))
		if e != nil || string(data) != "partial" {
			t.Fatal("lost partial output", e)
		}
		if _, e := os.Stat(filepath.Join(output, "after")); !os.IsNotExist(e) {
			t.Fatal("executed after failure", e)
		}
		data, e = os.ReadFile(filepath.Join(output, "word0.sh"))
		if e != nil || string(data) != source {
			t.Fatal("lost failed script", e)
		}
	})
	t.Run("write-failure-precedes-execution", func(t *testing.T) {
		output := t.TempDir()
		if err := os.Mkdir(filepath.Join(output, "pdf0.sh"), 0755); err != nil {
			t.Fatal(err)
		}
		_, err := runInstalledExample(context.Background(), shell, output, "pdf", 0, "printf bad > executed", os.Environ(), 60*time.Second)
		if err == nil {
			t.Fatal("directory script accepted")
		}
		if _, err := os.Stat(filepath.Join(output, "executed")); !os.IsNotExist(err) {
			t.Fatal("executed despite write failure", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		// A shell-only loop avoids leaving child processes behind in the regression.
		_, err := runInstalledExample(context.Background(), shell, t.TempDir(), "pdf", 0, "while :; do :; done", os.Environ(), 25*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout: %v", err)
		}
	})
}

func TestInstalledExampleSourceRoot(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "internal", "convert", "installed_skills_test.go")
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("package convert\n"), 0644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "source-link.go")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{source, alias} {
		got, err := installedExampleSourceRoot(file)
		if err != nil || got != want {
			t.Fatalf("source %s: root %q, want %q: %v", file, got, want, err)
		}
	}
}

func TestInstalledExampleDescendantTimeout(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	// The descendant inherits output but must neither extend the parent's
	// deadline nor be killed as a substitute for the original direct-child rule.
	source := "(sleep 1; printf survived > child-finished) &\nprintf 'parent-start\\n'\nwhile :; do :; done"
	start := time.Now()
	out, err := runInstalledExample(context.Background(), shell, output, "pdf", 0, source, os.Environ(), 100*time.Millisecond)
	elapsed := time.Since(start)
	t.Logf("direct-child wait=%v, error=%v", elapsed, err)
	// Wait for the bounded descendant before t.TempDir removes its directory.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, e := os.Stat(filepath.Join(output, "child-finished")); e == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, readErr := os.ReadFile(filepath.Join(output, "child-finished"))
	if readErr != nil || string(data) != "survived" {
		t.Fatalf("descendant did not survive: %q %v", data, readErr)
	}
	if !errors.Is(err, context.DeadlineExceeded) || elapsed >= 800*time.Millisecond || !strings.Contains(string(out), "parent-start") {
		t.Fatalf("direct-child deadline: elapsed=%v output=%q err=%v", elapsed, out, err)
	}
	files, err := filepath.Glob(filepath.Join(output, ".example-capture-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("visible capture files: %v %v", files, err)
	}
}
