package pdf

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestFillPreservesExistingDocumentInformation(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"full", "partial", "empty"} {
		c, err := Load(ctx, formPDF())
		if err != nil {
			t.Fatal(err)
		}
		info := types.Dict{"Title": types.StringLiteral("Original title"), "CustomKey": types.StringLiteral("Retained value")}
		if kind == "empty" {
			info = types.NewDict()
		}
		if kind == "full" {
			info["Producer"] = types.StringLiteral("Source producer")
			info["CreationDate"] = types.StringLiteral("D:20010102030405Z")
			info["ModDate"] = types.StringLiteral("D:20020203040506Z")
		}
		c.Info, err = c.IndRefForNewObject(info)
		if err != nil {
			t.Fatal(err)
		}
		c.PreserveInfoDict = true
		sourceBytes, err := Serialize(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		source := writeTemp(t, "source.pdf", sourceBytes)
		for _, flatten := range []bool{false, true} {
			output := filepath.Join(t.TempDir(), "filled.pdf")
			if _, err := Fill(ctx, source, output, map[string]string{"name": "Ada"}, flatten); err != nil {
				t.Fatal(err)
			}
			saved, _, err := LoadFile(ctx, output)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Info == nil {
				t.Fatal("lost existing information dictionary", kind)
			}
			got, err := saved.DereferenceDict(*saved.Info)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(info) {
				t.Fatalf("metadata keys changed: %v", got)
			}
			for key, want := range info {
				value := display(saved.XRefTable, got[key])
				if value == nil || *value != string(want.(types.StringLiteral)) {
					t.Fatalf("%s changed: %v", key, got[key])
				}
			}
		}
		after, err := os.ReadFile(source)
		if err != nil || !bytes.Equal(after, sourceBytes) {
			t.Fatal("source changed", err)
		}
	}
}

func TestFillDoesNotInventDocumentInformation(t *testing.T) {
	ctx := context.Background()
	sourceBytes := formPDF()
	original, err := Load(ctx, sourceBytes)
	if err != nil || original.Info != nil {
		t.Fatal("fixture must have no information dictionary", err)
	}
	source := writeTemp(t, "source.pdf", sourceBytes)
	for _, flatten := range []bool{false, true} {
		output := filepath.Join(t.TempDir(), "filled.pdf")
		if _, err := Fill(ctx, source, output, map[string]string{"name": "Ada"}, flatten); err != nil {
			t.Fatal(err)
		}
		saved, _, err := LoadFile(ctx, output)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Info != nil {
			t.Fatal("invented information dictionary", saved.Info)
		}
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(after, sourceBytes) {
		t.Fatal("source changed", err)
	}
}

func TestOmitGeneratedInfoOnlyChangesFinalTrailer(t *testing.T) {
	info := *types.NewIndirectRef(7, 0)
	prefix := "%PDF-1.7\n1 0 obj\n(stream text /Info 7 0 R trailer)\nendobj\nxref\n0 1\n0000000000 65535 f\n"
	trailer := "trailer\n<</Info 7 0 R/Root 1 0 R/Size 8>>\nstartxref\n80\n%%EOF\n"
	data, err := omitGeneratedInfo([]byte(prefix+trailer), info)
	want := prefix + "trailer\n<</Root 1 0 R/Size 8>>\nstartxref\n80\n%%EOF\n"
	if err != nil || string(data) != want {
		t.Fatalf("unexpected output %q: %v", data, err)
	}
	for _, invalid := range []string{
		"no trailer", "startxref\n0", "trailer\n<<>>\nstartxref\n0",
		"trailer\n<</Info 7 0 Rubbish>>\nstartxref\n0",
		"trailer\n<</Info 7 0 R/Info 7 0 R>>\nstartxref\n0",
	} {
		if _, err := omitGeneratedInfo([]byte(invalid), info); err == nil {
			t.Fatalf("accepted malformed writer output %q", invalid)
		}
	}
}
