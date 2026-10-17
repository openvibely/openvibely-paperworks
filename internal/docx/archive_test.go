package docx

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// Entire pinned reference DOCX library output, not a normalized ZIP or member-only check.
func TestReferenceDocumentArchive(t *testing.T) {
	data, err := os.ReadFile("testdata/archive.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Hash string `json:"archive_sha256"`
	}
	if err = json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	base := "../../testdata/document-examples/document_word/examples"
	data, err = os.ReadFile(base + "/report.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec Spec
	if err = json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return time.Date(2024, 2, 3, 4, 5, 7, 0, time.Local) }
	data, err = build(spec, base, clock)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != oracle.Hash {
		t.Fatalf("complete DOCX hash %s want %s", got, oracle.Hash)
	}
}
