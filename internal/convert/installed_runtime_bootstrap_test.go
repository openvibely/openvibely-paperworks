package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
)

// This records a deliberate architectural boundary, not Reference load equivalence.
// The compiled CLI does not execute sibling Reference modules at startup. Reference's
// runtime.load instead executes module bodies, keeps their side effects on failure,
// and propagates missing-module and initialization errors. Help AND a real read-only
// document operation below deliberately record that non-equivalence.
func TestCompiledBootstrapDoesNotLoadReferenceSiblings(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "paperworks")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/paperworks")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	binary, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	source := testsupport.OriginalPackageFixture(t, "runtime-word-fixtures.zip", "word", ".docx")
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	baselines := map[string][]byte{}
	for _, tc := range []struct {
		name, body string
		present    bool
	}{
		{"missing", "", false},
		{"syntax-error", "def broken(\n", true},
		{"bootstrap-error", "raise RuntimeError('bootstrap marker')\n", true},
		{"missing-dependency", "import paperworks_missing_bootstrap_dependency\n", true},
		{"side-effect", "from pathlib import Path\nPath(__file__).with_suffix('.executed').write_text('sibling')\nPath('bootstrap.executed').write_text('cwd')\n", true},
		{"partial-failure", "from pathlib import Path\nPath(__file__).with_suffix('.executed').write_text('sibling')\nPath('bootstrap.executed').write_text('cwd')\nraise RuntimeError('after side effect')\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			work := t.TempDir()
			installed := filepath.Join(dir, "paperworks")
			if err := os.WriteFile(installed, binary, 0700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"document_check.py", "word_edit.py", "test_skill_examples.py"} {
				if tc.present {
					if err := os.WriteFile(filepath.Join(dir, name), []byte(tc.body), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			// Cwd lookup must not rescue a missing sibling or execute a competing module.
			shadow := []byte("from pathlib import Path\nPath('cwd.executed').write_text('unexpected')\nraise RuntimeError('cwd shadow')\n")
			for _, name := range []string{"document_check.py", "word_edit.py", "test_skill_examples.py"} {
				if err := os.WriteFile(filepath.Join(work, name), shadow, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, operation := range []struct {
				name string
				args []string
			}{
				{"help", []string{"--help"}}, {"inspect", []string{"inspect", source}},
			} {
				cmd := exec.CommandContext(ctx, installed, operation.args...)
				cmd.Dir = work
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s: %v\n%s", operation.name, err, out)
				}
				if len(out) == 0 {
					t.Fatalf("empty %s output", operation.name)
				}
				if operation.name == "inspect" {
					var report map[string]json.RawMessage
					if err := json.Unmarshal(out, &report); err != nil || len(report["package_parts"]) == 0 || len(report["sections"]) == 0 {
						t.Fatalf("not a document inspection report: %v\n%s", err, out)
					}
				}
				if baseline := baselines[operation.name]; baseline == nil {
					baselines[operation.name] = append([]byte(nil), out...)
				} else if !bytes.Equal(out, baseline) {
					t.Fatalf("sibling files changed %s output", operation.name)
				}
			}
			after, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("inspection changed source", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if tc.present {
				want = 4
			}
			if len(entries) != want {
				t.Fatalf("unexpected startup artifacts: %v", entries)
			}
			workEntries, err := os.ReadDir(work)
			if err != nil || len(workEntries) != 3 {
				t.Fatalf("working directory modified: %v %v", workEntries, err)
			}
			for _, entry := range workEntries {
				data, err := os.ReadFile(filepath.Join(work, entry.Name()))
				if err != nil || !bytes.Equal(data, shadow) {
					t.Fatal("cwd shadow modified", err)
				}
			}
			for _, entry := range entries {
				if entry.Name() == "paperworks" {
					data, err := os.ReadFile(installed)
					if err != nil || !bytes.Equal(data, binary) {
						t.Fatal("installed binary modified")
					}
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil || string(data) != tc.body {
					t.Fatalf("sibling modified: %s: %v", entry.Name(), err)
				}
			}
		})
	}
}
