package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"github.com/openvibely/openvibely-paperworks/internal/testsupport"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// The original imported-comment sequence at the public saved-file boundary.
// The office-package test separately exercises the typed operation.
func TestReferenceImportedCommentsPublicWorkflow(t *testing.T) {
	dir := t.TempDir()
	specDir := t.TempDir()
	source := "../../testdata/fixtures/reference-comments.pptx"
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	type report struct {
		File     string                `json:"file"`
		Comments []office.SlideComment `json:"comments"`
	}
	invoke := func(input, output, spec string) (report, int, string) {
		t.Helper()
		args := []string{"slide-comments", input}
		if spec != "" {
			args = append(args, "--requirements", spec)
		}
		if output != "" {
			args = append(args, "--output", output)
		}
		var stdout, stderr bytes.Buffer
		code := Main(args, &stdout, &stderr)
		var r report
		if code == 0 {
			if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
				t.Fatal(err, stdout.String())
			}
		}
		return r, code, stderr.String()
	}
	r, code, msg := invoke(source, "", "")
	if code != 0 || len(r.Comments) == 0 {
		t.Fatal(code, msg, r)
	}
	root := r.Comments[0].ID
	current := source
	for _, action := range []string{"reopen", "reply", "react", "resolve"} {
		spec := map[string]any{"action": action, "thread_id": root}
		if action == "reply" {
			spec["author"] = "Second Reviewer"
			spec["text"] = "Updated"
		}
		if action == "react" {
			spec["author"] = "Second Reviewer"
			spec["reaction"] = "👍"
			spec["reply_id"] = r.Comments[len(r.Comments)-1].ID
		}
		path := writeSpec(t, specDir, action+".json", spec)
		out := testsupport.PresentationOutput(t, dir)
		before, err := os.ReadFile(current)
		if err != nil {
			t.Fatal(err)
		}
		r, code, msg = invoke(current, out, path)
		if code != 0 || len(r.Comments) == 0 {
			t.Fatal(action, code, msg, r)
		}
		if filepath.Clean(r.File) != filepath.Clean(out) {
			t.Fatal("wrong returned file", r.File, out)
		}
		if action == "reply" && r.Comments[len(r.Comments)-1].Text != "Updated" {
			t.Fatal(r)
		}
		if action == "react" {
			v := r.Comments[len(r.Comments)-1].Reactions
			if len(v) == 0 || v[0].Type != "👍" {
				t.Fatal(r)
			}
		}
		if action == "resolve" {
			for _, c := range r.Comments {
				if c.Status != "resolved" {
					t.Fatal(r)
				}
			}
		}
		after, err := os.ReadFile(current)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("input changed", err)
		}
		current = out
	}
	members := func(path string) (map[string][]byte, []string) {
		t.Helper()
		z, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		defer z.Close()
		result := map[string][]byte{}
		var names []string
		for _, f := range z.File {
			names = append(names, f.Name)
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			result[f.Name] = b
		}
		return result, names
	}
	old, names := members(source)
	saved, _ := members(current)
	for _, name := range names {
		b := old[name]
		if name == "ppt/authors.xml" || name == "ppt/comments/modernComment.xml" || name == "[Content_Types].xml" {
			continue
		}
		if !bytes.Equal(b, saved[name]) {
			t.Fatal("unrelated member changed", name)
		}
	}
	bad := writeSpec(t, specDir, "blocked.json", map[string]any{"action": "reply", "thread_id": root, "author": "X", "text": "Y"})
	absent := testsupport.PresentationOutput(t, dir)
	resolvedBefore, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	_, code, msg = invoke(current, absent, bad)
	if code == 0 || !strings.Contains(msg, "Reopen") {
		t.Fatal("wrong rejection", code, msg)
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatal("failed edit published output", err)
	}
	finalBefore, err := os.ReadFile(current)
	if err != nil || !bytes.Equal(resolvedBefore, finalBefore) {
		t.Fatal("rejected reply changed input", err)
	}
	_, code, _ = invoke(current, current, bad)
	if code == 0 {
		t.Fatal("existing destination accepted")
	}
	finalAfter, err := os.ReadFile(current)
	if err != nil || !bytes.Equal(finalBefore, finalAfter) {
		t.Fatal("existing output changed", err)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("original changed", err)
	}
}
