package wordedit

import (
	"bytes"
	"os"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

// Supplemental source-preservation helper contracts; all UNRUN during porting.
func TestReferenceOriginalGuardScope(t *testing.T) {
	originals := func(t *testing.T) []referenceOriginalSnapshot {
		t.Helper()
		var got []referenceOriginalSnapshot
		forReferenceOriginals(t, func(s referenceOriginalSnapshot) { got = append(got, s) })
		return got
	}
	t.Run("alternate-source-initialization", func(t *testing.T) {
		ensureReferenceOriginal(t)
		first := originals(t)
		if len(first) != 1 {
			t.Fatalf("originals: %d", len(first))
		}
		ensureReferenceOriginal(t)
		second := originals(t)
		if len(second) != 1 || second[0].path != first[0].path {
			t.Fatal("original replaced between operations")
		}
		data, err := os.ReadFile(first[0].path)
		if err != nil || !bytes.Equal(data, first[0].data) {
			t.Fatal("snapshot differs from fixture", err)
		}
	})
	t.Run("ancestor-and-multiple-fixtures", func(t *testing.T) {
		first, second := base(t), base(t)
		t.Run("chained-input", func(t *testing.T) {
			ensureReferenceOriginal(t)
			got := originals(t)
			if len(got) != 2 || got[0].path != first || got[1].path != second {
				t.Fatalf("ancestor originals: %#v", got)
			}
			if _, exists := referenceOriginals.Load(t.Name()); exists {
				t.Fatal("child hid ancestor originals behind a new fixture")
			}
		})
	})
	t.Run("sibling-isolation", func(t *testing.T) {
		if got := originals(t); len(got) != 0 {
			t.Fatalf("retained sibling fixtures: %#v", got)
		}
	})
}

// UNRUN: WordEditing.setUp's content and metadata must be in the bytes guarded
// by edit, including when the first operation uses an alternate source.
func TestReferenceOriginalGuardSetupFixture(t *testing.T) {
	ensureReferenceOriginal(t)
	var snapshots []referenceOriginalSnapshot
	forReferenceOriginals(t, func(s referenceOriginalSnapshot) { snapshots = append(snapshots, s) })
	if len(snapshots) != 1 {
		t.Fatalf("setup originals: %d", len(snapshots))
	}
	src := snapshots[0].path
	root := referenceXML(t, src, "word/document.xml")
	paragraphs := office.All(root, nsW, "p")
	if len(paragraphs) == 0 {
		t.Fatal("missing setup paragraph")
	}
	runs := office.All(paragraphs[0], nsW, "r")
	if len(runs) != 2 {
		t.Fatalf("setup runs: %d", len(runs))
	}
	referenceEqual(t, referenceTexts(runs[0]), "The old ")
	referenceEqual(t, referenceTexts(runs[1]), "price is $10.")
	referenceEqual(t, len(office.All(runs[0], nsW, "b")), 1)
	referenceEqual(t, len(office.All(runs[1], nsW, "i")), 1)
	tables := office.All(root, nsW, "tbl")
	if len(tables) != 1 {
		t.Fatalf("setup tables: %d", len(tables))
	}
	rows := office.All(tables[0], nsW, "tr")
	referenceEqual(t, len(rows), 2)
	for _, row := range rows {
		referenceEqual(t, len(office.All(row, nsW, "tc")), 2)
	}
	creators := office.All(referenceXML(t, src, "docProps/core.xml"), nsDC, "creator")
	if len(creators) != 1 {
		t.Fatalf("setup creators: %d", len(creators))
	}
	referenceEqual(t, creators[0].Text(), "Private author")
	data, err := os.ReadFile(src)
	if err != nil || !bytes.Equal(data, snapshots[0].data) {
		t.Fatal("guard did not snapshot complete setup fixture", err)
	}
}
