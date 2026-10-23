package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandHelpRoutesShareOutputAndSkipCommand(t *testing.T) {
	routes := [][]string{
		{"help", "convert"},
		{"convert", "--help"},
		{"convert", "-h"},
	}

	var want string
	for i, route := range routes {
		var stdout, stderr bytes.Buffer
		if code := Main(route, &stdout, &stderr); code != ExitOK {
			t.Fatalf("Main(%q) exit code = %d, want %d; stderr: %s", route, code, ExitOK, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("Main(%q) wrote to stderr: %s", route, stderr.String())
		}
		if i == 0 {
			want = stdout.String()
			if want == "" {
				t.Fatal("command help output is empty")
			}
			continue
		}
		if got := stdout.String(); got != want {
			t.Errorf("Main(%q) output differs from help convert\n got: %q\nwant: %q", route, got, want)
		}
	}
}

func TestTopLevelHelpAndUnknownCommandDiagnostic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"help"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("top-level help exit code = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout.String(), "Usage: paperworks COMMAND [ARGS] [--flags]") {
		t.Fatalf("top-level help missing usage: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("top-level help wrote to stderr: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Main([]string{"not-a-command"}, &stdout, &stderr); code != ExitUsage {
		t.Fatalf("unknown command exit code = %d, want %d", code, ExitUsage)
	}
	if got, want := stderr.String(), "paperworks: unknown command \"not-a-command\" (run 'paperworks help')\n"; got != want {
		t.Fatalf("unknown command diagnostic = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unknown command wrote to stdout: %s", stdout.String())
	}
}
