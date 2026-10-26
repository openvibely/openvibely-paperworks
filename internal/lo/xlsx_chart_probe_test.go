package lo

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

type refreshTestZIPEntry struct {
	name string
	data []byte
}

func TestXLSXChartMetadataProbeAndSafeFallback(t *testing.T) {
	chart, proven := xlsxChartParts("../../testdata/fixtures/reference-pie.xlsx")
	if !proven || !chart {
		t.Fatalf("chart workbook probe = (%v, %v), want (true, true)", chart, proven)
	}

	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.xlsx")
	writePlainRefreshWorkbook(t, plain)
	if chart, proven := xlsxChartParts(plain); !proven || chart {
		t.Fatalf("plain workbook probe = (%v, %v), want (false, true)", chart, proven)
	}

	badArchive := filepath.Join(dir, "malformed.xlsx")
	if err := os.WriteFile(badArchive, []byte("not a ZIP archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, proven := xlsxChartParts(badArchive); proven {
		t.Fatal("malformed archive was treated as proven chartless")
	}

	oversized := filepath.Join(dir, "oversized.xlsx")
	f, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxRefreshPackage + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, proven := xlsxChartParts(oversized); proven {
		t.Fatal("compressed-size limit violation was treated as proven chartless")
	}

	oversizedPart := filepath.Join(dir, "oversized-part.xlsx")
	writeOversizedRefreshTestMember(t, plain, oversizedPart)
	if _, proven := xlsxChartParts(oversizedPart); proven {
		t.Fatal("expanded-part limit violation was treated as proven chartless")
	}

	duplicate := filepath.Join(dir, "duplicate.xlsx")
	writeRefreshTestArchive(t, plain, duplicate, nil, nil, true)
	if _, proven := xlsxChartParts(duplicate); proven {
		t.Fatal("duplicate ZIP member was treated as proven chartless")
	}

	ambiguous := filepath.Join(dir, "ambiguous.xlsx")
	writeRefreshTestArchive(t, plain, ambiguous, map[string][]byte{"xl/_rels/workbook.xml.rels": []byte("<broken")}, nil, false)
	if _, proven := xlsxChartParts(ambiguous); proven {
		t.Fatal("malformed relationship metadata was treated as proven chartless")
	}

	corrupt := filepath.Join(dir, "corrupt.xlsx")
	writeRefreshTestArchive(t, plain, corrupt, nil, []refreshTestZIPEntry{{name: "probe/payload.dat", data: []byte("CRC-protected payload")}}, false)
	corruptRefreshTestZIPMember(t, corrupt, "probe/payload.dat")
	if _, proven := xlsxChartParts(corrupt); proven {
		t.Fatal("corrupt ZIP member was treated as proven chartless")
	}

	// Invalid archives and unsafe metadata must select the normal Office
	// fallback. The runner deliberately produces no status file; one invocation
	// proves the local byte-copy path was declined.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCUMENT_SOFFICE", exe)
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "malformed archive", path: badArchive},
		{name: "duplicate member", path: duplicate},
		{name: "ambiguous metadata", path: ambiguous},
		{name: "compressed size limit", path: oversized},
		{name: "expanded part limit", path: oversizedPart},
		{name: "corrupt member", path: corrupt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			runner := commandRunner(func(*exec.Cmd) ([]byte, error) {
				calls++
				return nil, nil
			})
			refreshErr := Refresh(context.WithValue(context.Background(), commandRunnerKey{}, runner), tc.path, filepath.Join(dir, "fallback-"+strings.ReplaceAll(tc.name, " ", "-")+".xlsx"), false)
			if refreshErr == nil || calls != 1 {
				t.Fatalf("fallback: error=%v runner_calls=%d, want one Office fallback", refreshErr, calls)
			}
			after, err := os.ReadFile(tc.path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("source changed during fallback: %v", err)
			}
		})
	}
}

func TestXLSXChartExRelationshipIsDetected(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "chart-ex.xlsx")
	mutations := map[string][]byte{}
	archive, err := zip.OpenReader("../../testdata/fixtures/reference-pie.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range archive.File {
		if part.Name != "[Content_Types].xml" && part.Name != "xl/drawings/_rels/drawing1.xml.rels" {
			continue
		}
		reader, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		if part.Name == "[Content_Types].xml" {
			data = bytes.Replace(data, []byte("application/vnd.openxmlformats-officedocument.drawingml.chart+xml"), []byte(chartExContentType), 1)
		} else {
			data = bytes.Replace(data, []byte("/chart\""), []byte("/chartEx\""), 1)
		}
		mutations[part.Name] = data
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	writeRefreshTestArchive(t, "../../testdata/fixtures/reference-pie.xlsx", src, mutations, nil, false)
	chart, proven := xlsxChartParts(src)
	if !proven || !chart {
		t.Fatalf("chartEx workbook probe = (%v, %v), want (true, true)", chart, proven)
	}
}

func TestRefreshWorkDetectionKeepsComplexXLSXOnOfficePath(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.xlsx")
	writePlainRefreshWorkbook(t, plain)
	archive, err := zip.OpenReader(plain)
	if err != nil {
		t.Fatal(err)
	}
	workbookXML := []byte(nil)
	for _, part := range archive.File {
		if part.Name == "xl/workbook.xml" {
			reader, err := part.Open()
			if err != nil {
				t.Fatal(err)
			}
			workbookXML, err = io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	definedNames := filepath.Join(dir, "defined-names.xlsx")
	nameXML := bytes.Replace(workbookXML, []byte("</workbook>"), []byte("<definedNames><definedName name=\"Example\">Sheet1!$A$1</definedName></definedNames></workbook>"), 1)
	writeRefreshTestArchive(t, plain, definedNames, map[string][]byte{"xl/workbook.xml": nameXML}, nil, false)
	if !xlsxHasRefreshWork(definedNames) {
		t.Fatal("defined-name workbook was not routed to Office")
	}
	assertRefreshFallsBackToOffice(t, definedNames)

	formulaPath := filepath.Join(dir, "formula.xlsx")
	formulaBook := excelize.NewFile()
	if err := formulaBook.SetCellFormula("Sheet1", "A1", "1+1"); err != nil {
		t.Fatal(err)
	}
	if err := formulaBook.SaveAs(formulaPath); err != nil {
		t.Fatal(err)
	}
	if err := formulaBook.Close(); err != nil {
		t.Fatal(err)
	}
	assertRefreshFallsBackToOffice(t, formulaPath)

	for _, name := range []string{"xl/tables/table1.xml", "xl/pivotCache/pivotCacheDefinition1.xml", "xl/externalLinks/externalLink1.xml"} {
		path := filepath.Join(dir, strings.ReplaceAll(name, "/", "-")+".xlsx")
		writeRefreshTestArchive(t, plain, path, nil, []refreshTestZIPEntry{{name: name, data: []byte("<part/>")}}, false)
		if !xlsxHasRefreshWork(path) {
			t.Errorf("complex workbook part %q was not routed to Office", name)
			continue
		}
		assertRefreshFallsBackToOffice(t, path)
	}
}

func assertRefreshFallsBackToOffice(t *testing.T, source string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCUMENT_SOFFICE", exe)
	calls := 0
	runner := commandRunner(func(*exec.Cmd) ([]byte, error) {
		calls++
		return nil, nil
	})
	err = Refresh(context.WithValue(context.Background(), commandRunnerKey{}, runner), source, filepath.Join(t.TempDir(), "fallback.xlsx"), false)
	if err == nil || calls != 1 {
		t.Fatalf("fallback for %s: error=%v runner_calls=%d, want one Office fallback", source, err, calls)
	}
}

func writePlainRefreshWorkbook(t *testing.T, path string) {
	t.Helper()
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "plain"); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeRefreshTestArchive(t *testing.T, source, destination string, mutations map[string][]byte, extra []refreshTestZIPEntry, duplicateWorkbook bool) {
	t.Helper()
	archive, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	file, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	var workbookCopy []byte
	for _, part := range archive.File {
		reader, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal(readErr, closeErr)
		}
		if changed := mutations[part.Name]; changed != nil {
			data = changed
		}
		if part.Name == "[Content_Types].xml" {
			for _, entry := range extra {
				if filepath.Ext(entry.name) != ".dat" {
					continue
				}
				data = bytes.Replace(data, []byte("</Types>"), []byte(`<Default Extension="dat" ContentType="application/octet-stream"/></Types>`), 1)
			}
		}
		member, err := writer.Create(part.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(data); err != nil {
			t.Fatal(err)
		}
		if part.Name == "xl/workbook.xml" {
			workbookCopy = bytes.Clone(data)
		}
	}
	if duplicateWorkbook {
		member, err := writer.Create("xl/workbook.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(workbookCopy); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range extra {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		member, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeOversizedRefreshTestMember(t *testing.T, source, destination string) {
	t.Helper()
	archive, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	file, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, part := range archive.File {
		reader, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		member, err := writer.Create(part.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(member, reader); err != nil {
			t.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
	}
	const inflated = uint64(128<<20) + 1
	header := &zip.FileHeader{Name: "xl/oversized.xml", Method: zip.Store, CompressedSize: 1, UncompressedSize: uint32(inflated), CompressedSize64: 1, UncompressedSize64: inflated}
	member, err := writer.CreateRaw(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func corruptRefreshTestZIPMember(t *testing.T, archivePath, name string) {
	t.Helper()
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	var offset int64 = -1
	for _, part := range archive.File {
		if part.Name == name {
			offset, err = part.DataOffset()
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if offset < 0 {
		t.Fatalf("ZIP member %q not found", name)
	}
	file, err := os.OpenFile(archivePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var value [1]byte
	if _, err := file.ReadAt(value[:], offset); err != nil {
		t.Fatal(err)
	}
	value[0] ^= 0xff
	if _, err := file.WriteAt(value[:], offset); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
