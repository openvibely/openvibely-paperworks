package convert

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReferenceSkillFileNewlines(t *testing.T) {
	for _, tc := range []struct{ name, newline string }{{"lf", "\n"}, {"crlf", "\r\n"}, {"cr", "\r"}} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "SKILL.md")
			raw := []byte(strings.Join([]string{"```sh", `printf probe`, "```"}, tc.newline))
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			text, err := readSkillText(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := shellBlocks(t, text); !reflect.DeepEqual(got, []string{`printf probe`}) {
				t.Fatalf("blocks=%q", got)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(raw) {
				t.Fatal("source changed")
			}
		})
	}
}
