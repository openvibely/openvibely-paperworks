package opc

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type closeErrorFile struct {
	*os.File
	closeErr error
}

func (f closeErrorFile) Close() error {
	if err := f.File.Close(); err != nil {
		return err
	}
	return f.closeErr
}

func TestWriteNewRemovesOutputWhenCloseFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial-output")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	closeErr := errors.New("simulated close failure")
	if err := writeNewContents(path, closeErrorFile{File: file, closeErr: closeErr}, []byte("partial output")); !errors.Is(err, closeErr) {
		t.Fatalf("writeNewContents error = %v, want %v", err, closeErr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("output remains after close failure: %v", err)
	}
}

func TestSetXMLWithUTF8BOMHasOneDeclaration(t *testing.T) {
	for _, declaration := range []string{"", `<?xml version="1.0" encoding="utf-8"?>`} {
		input := append([]byte{0xef, 0xbb, 0xbf}, []byte(declaration+`<Types xmlns="`+NSCT+`"><Default Extension="xml" ContentType="application/xml"/></Types>`)...)
		d, err := ParseXML(input)
		if err != nil {
			t.Fatal(err)
		}
		d.Root().CreateAttr("checked", "yes")
		p := &Package{index: map[string]*Part{}}
		if err := p.SetXML("[Content_Types].xml", d); err != nil {
			t.Fatal(err)
		}
		data := p.Get("[Content_Types].xml")
		if bytes.Count(data, []byte("<?xml")) != 1 || bytes.Contains(data, []byte{0xef, 0xbb, 0xbf}) {
			t.Fatalf("invalid declaration: %q", data)
		}
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			_, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestStoredMemberRewritePreservesBytes(t *testing.T) {
	p, err := Open("../../testdata/fixtures/reference-treemap.pptx")
	if err != nil {
		t.Fatal(err)
	}
	stored := false
	for _, part := range p.Parts {
		stored = stored || part.Header.Method == zip.Store
	}
	if !stored {
		t.Fatal("fixture must exercise stored ZIP members")
	}
	data, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	q, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range p.Parts {
		if !bytes.Equal(part.Data, q.Get(part.Name)) {
			t.Fatal("changed part", part.Name)
		}
	}
	for _, part := range q.Parts {
		if part.Header.Method == zip.Store && part.Header.Flags&8 != 0 {
			t.Fatal("stored streaming member", part.Name)
		}
	}
}

func TestUnusedName(t *testing.T) {
	p := &Package{index: map[string]*Part{}}
	if got := p.UnusedName("ppt/media/fill", ".png"); got != "ppt/media/fill1.png" {
		t.Fatalf("empty package: %s", got)
	}
	p.Set("ppt/media/fill1.png", nil)
	p.Set("ppt/media/fill2.png", nil)
	p.Set("ppt/media/fill4.png", nil)
	if got := p.UnusedName("ppt/media/fill", ".png"); got != "ppt/media/fill3.png" {
		t.Fatalf("gap: %s", got)
	}
	p.Set("ppt/media/fill3.png", nil)
	if got := p.UnusedName("ppt/media/fill", ".png"); got != "ppt/media/fill5.png" {
		t.Fatalf("skip existing: %s", got)
	}
	if got := p.UnusedName("ppt/media/fill", ".jpeg"); got != "ppt/media/fill1.jpeg" {
		t.Fatalf("suffix distinguishes names: %s", got)
	}
}
