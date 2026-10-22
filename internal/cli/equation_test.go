package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestEquationCLIByteOrderMark(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.txt")
	bom := filepath.Join(dir, "bom.txt")
	crlf := filepath.Join(dir, "crlf.txt")
	for p, s := range map[string]string{plain: "\\frac{a}{b}\n", bom: "\xef\xbb\xbf\\frac{a}{b}\n", crlf: "\xef\xbb\xbfx^2\r\n"} {
		if e := os.WriteFile(p, []byte(s), 0600); e != nil {
			t.Fatal(e)
		}
	}
	a := runCLI(t, "equation", plain, filepath.Join(dir, "plain.png"))
	b := runCLI(t, "equation", bom, filepath.Join(dir, "bom.png"))
	if a["width"] != b["width"] || a["height"] != b["height"] {
		t.Fatalf("dimensions differ: %v %v", a, b)
	}
	pa, _ := os.ReadFile(filepath.Join(dir, "plain.png"))
	pb, _ := os.ReadFile(filepath.Join(dir, "bom.png"))
	if len(pa) == 0 || !bytes.Equal(pa, pb) {
		t.Fatal("BOM changed PNG")
	}
	runCLI(t, "equation", crlf, filepath.Join(dir, "crlf.png"))
	for i, s := range []string{"\xef\xbb\xbf", "\xef\xbb\xbf\n", "\xef\xbb\xbf \r\n"} {
		src := filepath.Join(dir, "empty"+string(rune('0'+i))+".txt")
		dst := filepath.Join(dir, "empty"+string(rune('0'+i))+".png")
		if e := os.WriteFile(src, []byte(s), 0600); e != nil {
			t.Fatal(e)
		}
		var stdout, stderr bytes.Buffer
		if Main([]string{"equation", src, dst}, &stdout, &stderr) == 0 || !strings.Contains(stderr.String(), "nonempty UTF-8 TeX expression") {
			t.Fatalf("%q accepted: %s", s, stderr.String())
		}
		if _, e := os.Lstat(dst); !os.IsNotExist(e) {
			t.Fatalf("%q created destination", s)
		}
	}
}
