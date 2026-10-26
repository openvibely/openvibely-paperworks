package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnknownOptionIsUsageErrorBeforeCommandExecution(t *testing.T) {
	output := filepath.Join(t.TempDir(), "rotated.pdf")
	var stdout, stderr bytes.Buffer

	code := Main([]string{"pdf-rotate", "missing.pdf", "--degress", "180", "--output", output}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("Main exit code = %d, want %d; stderr: %s", code, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown option --degress") {
		t.Fatalf("stderr does not identify misspelled option: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "paperworks pdf-rotate INPUT.pdf") {
		t.Fatalf("stderr does not include command usage: %s", stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output exists after rejecting unknown option: stat err = %v", err)
	}
}

func TestParseDeclaredValueAndBooleanOptions(t *testing.T) {
	a, err := parse(commands["fill"], []string{
		"input.pdf",
		"--values", "values.json",
		"--output=output.pdf",
		"--flatten=false",
		"--flatten",
		"--strict-choices=true",
	})
	if err != nil {
		t.Fatalf("parse valid options: %v", err)
	}
	if a.Flags["values"] != "values.json" || a.Flags["output"] != "output.pdf" {
		t.Fatalf("value options parsed incorrectly: %#v", a.Flags)
	}
	if !a.Bools["flatten"] || !a.Bools["strict-choices"] {
		t.Fatalf("boolean options parsed incorrectly: %#v", a.Bools)
	}

	rotateArgs, err := parse(commands["pdf-rotate"], []string{"input.pdf", "--degrees", "180", "--pages=1,3", "--output", "rotated.pdf"})
	if err != nil {
		t.Fatalf("parse valid rotation options: %v", err)
	}
	if rotateArgs.Flags["degrees"] != "180" || rotateArgs.Flags["pages"] != "1,3" || rotateArgs.Flags["output"] != "rotated.pdf" {
		t.Fatalf("rotation options parsed incorrectly: %#v", rotateArgs.Flags)
	}

	if _, err := parse(commands["pdf-rotate"], []string{"input.pdf", "--degrees", "180", "--degrees", "270"}); err == nil || !strings.Contains(err.Error(), "--degrees given more than once") {
		t.Fatalf("duplicate value option error = %v, want duplicate-option usage error", err)
	}
}
