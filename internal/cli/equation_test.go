package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEquationCLI(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "eq.txt")
	if e := os.WriteFile(src, []byte(`\frac{1}{2}`), 0600); e != nil {
		t.Fatal(e)
	}
	display := runCLI(t, "equation", src, filepath.Join(dir, "display.png"))
	inline := runCLI(t, "equation", src, filepath.Join(dir, "inline.png"), "--inline")
	if inline["height"].(float64) >= display["height"].(float64) {
		t.Fatal("inline style ignored")
	}
	if display["reference_verified"] != false {
		t.Fatal("false equivalence claim")
	}
	var stdout, stderr bytes.Buffer
	if Main([]string{"equation", src, filepath.Join(dir, "display.png")}, &stdout, &stderr) == 0 {
		t.Fatal("overwrite accepted")
	}
	if stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatal("error streams")
	}
}
