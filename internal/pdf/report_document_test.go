package pdf

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedReportDocuments(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1706933107")
	var fixture struct {
		Cases []struct {
			Name   string
			Spec   CreateSpec
			SHA256 string
		}
	}
	raw, err := os.ReadFile("testdata/reportlab-documents.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("../../testdata/document-examples/document_pdf/examples/report.json")
	if err != nil {
		t.Fatal(err)
	}
	var installed CreateSpec
	if err = json.Unmarshal(raw, &installed); err != nil {
		t.Fatal(err)
	}
	fixture.Cases = append(fixture.Cases, struct {
		Name   string
		Spec   CreateSpec
		SHA256 string
	}{"installed", installed, "fd14ccbd088076eb3de7f887ecccbc78c2f58c8077c7a7b4fc44d4a54078d710"})
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "report.pdf")
			if err := Create(context.Background(), c.Spec, "", p); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != c.SHA256 {
				t.Fatalf("whole PDF %s want %s", got, c.SHA256)
			}
		})
	}
}

func TestReportClockFailureDoesNotPublish(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "invalid")
	raw, err := os.ReadFile("../../testdata/document-examples/document_pdf/examples/report.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec CreateSpec
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "report.pdf")
	if err = Create(context.Background(), spec, "", p); err == nil {
		t.Fatal("invalid clock accepted")
	}
	if _, err = os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("output published: %v", err)
	}
}
