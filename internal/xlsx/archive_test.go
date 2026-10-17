package xlsx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPinnedSpreadsheetArchive(t *testing.T) {
	b, e := os.ReadFile("../../testdata/document-examples/document_spreadsheets/examples/report.json")
	if e != nil {
		t.Fatal(e)
	}
	var s Spec
	if e = json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "report.xlsx")
	clock := func() time.Time { return time.Date(2024, 2, 3, 4, 5, 7, 0, time.UTC) }
	if e = createWithClock(context.Background(), s, "../../testdata/document-examples/document_spreadsheets/examples", out, clock); e != nil {
		t.Fatal(e)
	}
	b, e = os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}

	data, e := os.ReadFile("testdata/archive-reference.json")
	if e != nil {
		t.Fatal(e)
	}
	var ref struct {
		Hash string `json:"archive_sha256"`
	}
	if e = json.Unmarshal(data, &ref); e != nil {
		t.Fatal(e)
	}
	if h := fmt.Sprintf("%x", sha256.Sum256(b)); h != ref.Hash {
		t.Fatalf("whole archive %s want %s", h, ref.Hash)
	}
}

// Metadata must reflect the separate construction and save clocks, while
// package extensions must survive the new-writer ordering path.
func TestSpreadsheetArchiveClocksAndExtensions(t *testing.T) {
	p, err := opc.Read(mustArchiveFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	p.Set("custom/retained.bin", []byte("unchanged"))
	created := time.Date(2024, 2, 3, 23, 59, 59, 0, time.UTC)
	saved := created.Add(2 * time.Second)
	raw, err := spreadsheetArchive(p, created, func() time.Time { return saved })
	if err != nil {
		t.Fatal(err)
	}
	q, err := opc.Read(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(q.Get("custom/retained.bin")) != "unchanged" {
		t.Fatal("extension lost")
	}
	core := string(q.Get("docProps/core.xml"))
	if !strings.Contains(core, "2024-02-03T23:59:59Z</dcterms:created>") || !strings.Contains(core, "2024-02-04T00:00:01Z</dcterms:modified>") {
		t.Fatal(core)
	}
}
func mustArchiveFixture(t *testing.T) []byte {
	t.Helper()
	p := &opc.Package{}
	// Initialize the package through its reader, then exercise insertion.
	raw, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
