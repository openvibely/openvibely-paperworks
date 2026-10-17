package wordedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferenceNamedEditRouting(t *testing.T) {
	source := base(t)
	root := filepath.Dir(source)
	spec := map[string]any{"operation": "metadata", "clear": []any{"author"}}
	first, err := referenceNamedEdit(t, root, "named", "", spec)
	if err != nil {
		t.Fatal(err)
	}
	if first != filepath.Join(root, "named.docx") {
		t.Fatal(first)
	}
	if _, err = referenceNamedEdit(t, root, "named", "", spec); err == nil || !strings.Contains(err.Error(), "new file") {
		t.Fatal("repeated destination", err)
	}
	if _, err = referenceNamedEdit(t, root, "invalid", "", map[string]any{"operation": "unknown"}); err == nil {
		t.Fatal("invalid accepted")
	}
	if _, err = os.Stat(filepath.Join(root, "invalid.docx")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	second, err := referenceNamedEdit(t, root, "alternate", first, map[string]any{"operation": "metadata", "clear": []any{"title"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(part(t, second, "docProps/core.xml"), "Private author") {
		t.Fatal("alternate routing")
	}
}

func TestReferenceNamedEditAbsoluteDestination(t *testing.T) {
	source := base(t)
	root := filepath.Dir(source)
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "named")
	out, err := referenceNamedEdit(t, root, name, "", map[string]any{"operation": "metadata", "clear": []any{"author"}})
	if err != nil {
		t.Fatal(err)
	}
	if out != name+".docx" {
		t.Fatalf("output %q, want %q", out, name+".docx")
	}
	if strings.Contains(part(t, out, "docProps/core.xml"), "Private author") {
		t.Fatal("author not cleared")
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("source root entries changed")
	}
	for i := range before {
		if before[i].Name() != after[i].Name() {
			t.Fatal("source root entries changed")
		}
	}
}

// Pathlib preserves parent components: a missing intermediate directory cannot
// be erased before the filesystem resolves the destination.
func TestReferenceNamedEditParentDestination(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "existing"}[exists], func(t *testing.T) {
			source := base(t)
			root := filepath.Dir(source)
			if exists {
				if err := os.Mkdir(filepath.Join(root, "middle"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			out, err := referenceNamedEdit(t, root, "middle/../named", "", map[string]any{"operation": "metadata", "clear": []any{"author"}})
			if exists {
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(part(t, out, "docProps/core.xml"), "Private author") {
					t.Fatal("author not cleared")
				}
			} else {
				if !os.IsNotExist(err) {
					t.Fatalf("want missing intermediate error, got %v", err)
				}
			}
			_, statErr := os.Stat(filepath.Join(root, "named.docx"))
			if exists && statErr != nil {
				t.Fatal(statErr)
			}
			if !exists && !os.IsNotExist(statErr) {
				t.Fatalf("output published: %v", statErr)
			}
			entries, e := os.ReadDir(root)
			if e != nil {
				t.Fatal(e)
			}
			want := 1
			if exists {
				want = 3
			}
			if len(entries) != want {
				t.Fatalf("entries=%v", entries)
			}
		})
	}
}

func TestReferenceNamedEditDotDestination(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"./named", "named.docx"},
		{"nested//./named", "nested/named.docx"},
		{"nested/.././named", "nested/../named.docx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := base(t)
			root := filepath.Dir(source)
			if err := os.Mkdir(root+"/nested", 0700); err != nil {
				t.Fatal(err)
			}
			out, err := referenceNamedEdit(t, root, tc.name, "", map[string]any{"operation": "metadata", "clear": []any{"author"}})
			if err != nil {
				t.Fatal(err)
			}
			if out != root+"/"+tc.want {
				t.Fatalf("returned %q want %q", out, root+"/"+tc.want)
			}
			if strings.Contains(part(t, out, "docProps/core.xml"), "Private author") {
				t.Fatal("author not cleared")
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			want := 3
			if tc.want == "nested/named.docx" {
				want = 2
			}
			if len(entries) != want {
				t.Fatalf("entries=%v", entries)
			}
		})
	}
}
