// Package cli implements the paperworks command-line contract: JSON results on
// stdout, diagnostics on stderr, and documented exit codes.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/convert"
)

var Version = "0.1.0-dev"

const (
	ExitOK         = 0
	ExitFailure    = 1
	ExitUsage      = 2
	ExitDependency = 3
	ExitVerify     = 4
)

// UsageError selects exit code 2.
type UsageError struct{ Msg string }

func (e UsageError) Error() string { return e.Msg }

// DependencyError selects exit code 3 (for example, LibreOffice missing).
type DependencyError struct{ Msg string }

func (e DependencyError) Error() string { return e.Msg }

// VerifyError selects exit code 4: the command ran but the document failed checks.
type VerifyError struct {
	Msg    string
	Result any
}

func (e VerifyError) Error() string { return e.Msg }

func usagef(format string, a ...any) error { return UsageError{fmt.Sprintf(format, a...)} }

type Args struct {
	Pos   []string
	Flags map[string]string
	Bools map[string]bool
}

func (a Args) Str(name string) string { return a.Flags[name] }

func (a Args) Path(name string) string {
	if v := a.Flags[name]; v != "" {
		return absPath(v)
	}
	return ""
}

func (a Args) Int(name string, def int) (int, error) {
	v, ok := a.Flags[name]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, usagef("--%s must be an integer", name)
	}
	return n, nil
}

func (a Args) Float(name string) (float64, bool, error) {
	v, ok := a.Flags[name]
	if !ok {
		return 0, false, nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false, usagef("--%s must be a number", name)
	}
	return n, true, nil
}

func (a Args) Require(names ...string) error {
	for _, n := range names {
		if a.Flags[n] == "" {
			return usagef("missing required --%s", n)
		}
	}
	return nil
}

func (a Args) JSON(name string, into any) error {
	p := a.Path(name)
	if p == "" {
		return usagef("missing required --%s JSON file", name)
	}
	return readJSONFile(p, into)
}

func readJSONFile(p string, into any) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (64<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 64<<20 {
		return fmt.Errorf("%s: JSON file exceeds 64 MiB", p)
	}
	if err := json.Unmarshal(data, into); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) && syntax.Offset > 0 && syntax.Offset <= int64(len(data)) {
			tail := string(data[syntax.Offset-1:])
			if strings.HasPrefix(tail, "NaN") || strings.HasPrefix(tail, "Infinity") {
				return fmt.Errorf("%s: numbers must be finite: %w", p, err)
			}
		}
		return fmt.Errorf("%s: invalid JSON: %w", p, err)
	}
	return nil
}

func absPath(p string) string {
	if p == "-" {
		return p
	}
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

type Command struct {
	Name     string
	Summary  string
	Usage    string
	BoolOpts []string
	Run      func(ctx context.Context, a Args) (any, error)
}

var commands = map[string]*Command{}

func register(c *Command) { commands[c.Name] = c }

func parse(c *Command, raw []string) (Args, error) {
	a := Args{Flags: map[string]string{}, Bools: map[string]bool{}}
	isBool := map[string]bool{}
	for _, b := range c.BoolOpts {
		isBool[b] = true
	}
	for i := 0; i < len(raw); i++ {
		s := raw[i]
		if s == "--" {
			a.Pos = append(a.Pos, raw[i+1:]...)
			break
		}
		if strings.HasPrefix(s, "--") && len(s) > 2 {
			name, val, hasVal := strings.Cut(s[2:], "=")
			if isBool[name] {
				if hasVal {
					b, err := strconv.ParseBool(val)
					if err != nil {
						return a, usagef("--%s expects true/false", name)
					}
					a.Bools[name] = b
				} else {
					a.Bools[name] = true
				}
				continue
			}
			if !hasVal {
				if i+1 >= len(raw) {
					return a, usagef("--%s requires a value", name)
				}
				i++
				val = raw[i]
			}
			if _, dup := a.Flags[name]; dup {
				return a, usagef("--%s given more than once", name)
			}
			a.Flags[name] = val
			continue
		}
		a.Pos = append(a.Pos, s)
	}
	return a, nil
}

func Main(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
		if len(argv) > 1 {
			if c := commands[argv[1]]; c != nil {
				printCommandHelp(stdout, c)
				return ExitOK
			}
		}
		printHelp(stdout)
		if len(argv) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	if argv[0] == "--version" {
		argv[0] = "version"
	}
	c := commands[argv[0]]
	if c == nil {
		fmt.Fprintf(stderr, "paperworks: unknown command %q (run 'paperworks help')\n", argv[0])
		return ExitUsage
	}
	for _, s := range argv[1:] {
		if s == "--help" || s == "-h" {
			printCommandHelp(stdout, c)
			return ExitOK
		}
	}
	a, err := parse(c, argv[1:])
	if err == nil {
		var result any
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		timeout := 30 * time.Minute
		if v := os.Getenv("PAPERWORKS_TIMEOUT"); v != "" {
			if d, perr := time.ParseDuration(v); perr == nil && d > 0 {
				timeout = d
			}
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		result, err = c.Run(ctx, a)
		if err == nil {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			if encErr := enc.Encode(result); encErr != nil {
				fmt.Fprintln(stderr, "paperworks:", encErr)
				return ExitFailure
			}
			return ExitOK
		}
	}
	var ue UsageError
	var de DependencyError
	var ve VerifyError
	switch {
	case errors.As(err, &ue):
		fmt.Fprintf(stderr, "paperworks %s: %s\n\n%s\n", c.Name, ue.Msg, c.Usage)
		return ExitUsage
	case errors.As(err, &de):
		fmt.Fprintf(stderr, "paperworks %s: missing dependency: %s\n", c.Name, de.Msg)
		return ExitDependency
	case errors.As(err, &ve):
		if ve.Result != nil {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			enc.Encode(ve.Result)
		}
		fmt.Fprintf(stderr, "paperworks %s: verification failed: %s\n", c.Name, ve.Msg)
		return ExitVerify
	}
	fmt.Fprintf(stderr, "paperworks %s: %s\n", c.Name, err)
	return ExitFailure
}

func printCommandHelp(w io.Writer, c *Command) {
	fmt.Fprintf(w, "%s\n\n%s\n", c.Summary, c.Usage)
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, "paperworks %s — create, inspect, edit, validate and render PDF/DOCX/XLSX/PPTX documents.\n\n", Version)
	fmt.Fprintln(w, "Usage: paperworks COMMAND [ARGS] [--flags]   (paperworks help COMMAND for details)")
	fmt.Fprintln(w, "Results are JSON on stdout; diagnostics on stderr.")
	fmt.Fprintln(w, "Exit codes: 0 ok, 1 failure, 2 usage, 3 missing dependency, 4 verification failed.")
	fmt.Fprintln(w)
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "  %-20s %s\n", n, commands[n].Summary)
	}
}

// input returns the single positional input as an absolute, existing path.
func input(a Args, n int) ([]string, error) {
	if len(a.Pos) != n {
		return nil, usagef("expected %d positional argument(s), got %d", n, len(a.Pos))
	}
	out := make([]string, n)
	for i, p := range a.Pos {
		out[i] = absPath(p)
	}
	return out, nil
}

func existing(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%s is a directory", p)
	}
	return nil
}

// newDir creates a fresh output directory; existing directories are refused so
// stale results never look like fresh ones.
func newDir(p string) error {
	if p == "" {
		return usagef("--output must name a new directory")
	}
	return convert.NewOutputDir(p)
}

func newFile(out, in string) error {
	if out == "" {
		return usagef("--output must name a new file")
	}
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("output already exists: %s: %w", out, os.ErrExist)
	}
	if in != "" && filepath.Clean(out) == filepath.Clean(in) {
		return fmt.Errorf("output must differ from the input")
	}
	return nil
}
