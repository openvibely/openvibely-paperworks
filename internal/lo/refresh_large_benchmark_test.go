package lo

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"github.com/xuri/excelize/v2"
)

const refreshLargeBenchmarkGate = "PAPERWORKS_REFRESH_LARGE_BENCHMARK"
const refreshLargeBenchmarkChild = "PAPERWORKS_REFRESH_LARGE_BENCHMARK_CHILD"

type refreshBenchmarkResult struct {
	mode   string
	median time.Duration
	p95    time.Duration
	alloc  uint64
	rss    uint64
}

// TestRefreshBenchmarkLargeExpandedPlainXLSX compares 30 operations using the
// previous full-OPC-open path and Refresh's metadata-only chartless path. It is
// opt-in because the fixture expands to 180 MiB and the baseline retains that
// data while parsing it. Run with PAPERWORKS_REFRESH_LARGE_BENCHMARK=1.
func TestRefreshBenchmarkLargeExpandedPlainXLSX(t *testing.T) {
	if mode := os.Getenv(refreshLargeBenchmarkChild); mode != "" {
		runRefreshLargeBenchmarkChild(t, mode)
		return
	}
	if os.Getenv(refreshLargeBenchmarkGate) != "1" {
		t.Skip("set PAPERWORKS_REFRESH_LARGE_BENCHMARK=1 to run the 30-operation large-workbook comparison")
	}

	root := t.TempDir()
	src := filepath.Join(root, "plain-expanded.xlsx")
	createExpandedPlainWorkbook(t, src)
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := refreshArchiveExpandedSize(src)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= 32<<20 || expanded < 150<<20 || expanded > 200<<20 {
		t.Fatalf("benchmark fixture size compressed=%d expanded=%d; want <32 MiB compressed and 150–200 MiB expanded", info.Size(), expanded)
	}
	t.Logf("fixture compressed_bytes=%d expanded_bytes=%d", info.Size(), expanded)

	results := make([]refreshBenchmarkResult, 0, 2)
	for _, mode := range []string{"before", "after"} {
		outDir := filepath.Join(root, mode)
		if err := os.MkdirAll(outDir, 0700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestRefreshBenchmarkLargeExpandedPlainXLSX$")
		cmd.Env = append(os.Environ(), refreshLargeBenchmarkChild+"="+mode, "PAPERWORKS_REFRESH_LARGE_BENCHMARK_INPUT="+src, "PAPERWORKS_REFRESH_LARGE_BENCHMARK_OUTPUT="+outDir)
		output, err := cmd.CombinedOutput()
		t.Logf("%s child output:\n%s", mode, strings.TrimSpace(string(output)))
		if err != nil {
			t.Fatalf("%s benchmark child: %v", mode, err)
		}
		var parsed refreshBenchmarkResult
		for _, line := range strings.Split(string(output), "\n") {
			if !strings.HasPrefix(line, "REFRESH_BENCH_RESULT ") {
				continue
			}
			parsed, err = parseRefreshBenchmarkResult(line)
			if err != nil {
				t.Fatal(err)
			}
		}
		if parsed.mode == "" {
			t.Fatalf("%s benchmark child did not report measurements", mode)
		}
		parsed.rss = processPeakRSSBytes(cmd.ProcessState)
		results = append(results, parsed)
	}

	before, after := results[0], results[1]
	t.Logf("comparison runs=30 compressed_bytes=%d expanded_bytes=%d before_median=%s before_p95=%s before_B_op=%d before_peak_RSS=%d after_median=%s after_p95=%s after_B_op=%d after_peak_RSS=%d", info.Size(), expanded, before.median, before.p95, before.alloc, before.rss, after.median, after.p95, after.alloc, after.rss)
	if after.alloc*10 > before.alloc*3 {
		t.Errorf("allocated bytes fell by less than 70%%: before=%d after=%d", before.alloc, after.alloc)
	}
	if before.rss < after.rss+100<<20 {
		t.Errorf("peak RSS fell by less than 100 MiB: before=%d after=%d", before.rss, after.rss)
	}
	if after.median*100 > before.median*105 {
		t.Errorf("median latency regressed by more than 5%%: before=%s after=%s", before.median, after.median)
	}
}

func runRefreshLargeBenchmarkChild(t *testing.T, mode string) {
	t.Helper()
	if mode != "before" && mode != "after" {
		t.Fatalf("invalid benchmark child mode %q", mode)
	}
	src := os.Getenv("PAPERWORKS_REFRESH_LARGE_BENCHMARK_INPUT")
	outDir := os.Getenv("PAPERWORKS_REFRESH_LARGE_BENCHMARK_OUTPUT")
	if src == "" || outDir == "" {
		t.Fatal("benchmark child input and output paths are required")
	}
	t.Setenv("DOCUMENT_SOFFICE", filepath.Join(t.TempDir(), "missing-soffice"))
	sourceBefore, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	const samples = 30
	times := make([]time.Duration, 0, samples)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < samples; i++ {
		dst := filepath.Join(outDir, fmt.Sprintf("refresh-%02d.xlsx", i))
		started := time.Now()
		var err error
		if mode == "before" {
			err = refreshXLSXWithoutOfficeBeforeOptimization(src, dst)
		} else {
			err = Refresh(context.Background(), src, dst, false)
		}
		if err != nil {
			t.Fatalf("refresh %d: %v", i, err)
		}
		elapsed := time.Since(started)
		times = append(times, elapsed)
		info, err := os.Stat(dst)
		if err != nil || info.Size() == 0 {
			t.Fatalf("refresh %d output missing or empty: %v", i, err)
		}
	}
	runtime.ReadMemStats(&after)
	sourceBytes, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	outputBytes, err := os.ReadFile(filepath.Join(outDir, fmt.Sprintf("refresh-%02d.xlsx", samples-1)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sourceBefore, sourceBytes) {
		t.Fatal("chartless refresh changed the source archive")
	}
	if !bytes.Equal(sourceBytes, outputBytes) {
		t.Fatal("chartless refresh output differs from source bytes")
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	median := (times[14] + times[15]) / 2
	p95 := times[int(math.Ceil(.95*float64(samples)))-1]
	allocPerOp := (after.TotalAlloc - before.TotalAlloc) / samples
	fmt.Printf("REFRESH_BENCH_RESULT mode=%s median_ns=%d p95_ns=%d bytes_per_op=%d\n", mode, median.Nanoseconds(), p95.Nanoseconds(), allocPerOp)
}

// refreshXLSXWithoutOfficeBeforeOptimization reproduces the former chartless
// branch: expand and retain all OPC parts to prove there are no charts, then
// copy the source archive unchanged.
func refreshXLSXWithoutOfficeBeforeOptimization(src, dst string) error {
	if xlsxHasRefreshWork(src) {
		return fmt.Errorf("benchmark workbook unexpectedly needs office refresh")
	}
	pkg, err := opc.Open(src)
	if err != nil {
		return err
	}
	charts, err := office.ChartParts(pkg)
	if err != nil {
		return err
	}
	if len(charts) != 0 {
		return fmt.Errorf("benchmark workbook unexpectedly contains charts")
	}
	return copyFileExclusive(src, dst)
}

func createExpandedPlainWorkbook(t *testing.T, destination string) {
	t.Helper()
	base := filepath.Join(t.TempDir(), "base.xlsx")
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "plain workbook"); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(base); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(base)
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
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal("read base workbook member", readErr, closeErr)
		}
		if part.Name == "[Content_Types].xml" {
			doc, err := opc.ParseXML(data)
			if err != nil {
				t.Fatal(err)
			}
			defaultType := doc.Root().CreateElement("Default")
			defaultType.CreateAttr("Extension", "dat")
			defaultType.CreateAttr("ContentType", "application/octet-stream")
			data, err = doc.WriteToBytes()
			if err != nil {
				t.Fatal(err)
			}
		}
		member, err := writer.Create(part.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	const expandedPart = 60 << 20
	payload := bytes.Repeat([]byte{'x'}, 1<<20)
	for i := 0; i < 3; i++ {
		member, err := writer.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("benchmark/expanded-%d.dat", i+1), Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		for written := 0; written < expandedPart; written += len(payload) {
			if _, err := member.Write(payload); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func refreshArchiveExpandedSize(src string) (uint64, error) {
	archive, err := zip.OpenReader(src)
	if err != nil {
		return 0, err
	}
	defer archive.Close()
	var expanded uint64
	for _, part := range archive.File {
		expanded += part.UncompressedSize64
	}
	return expanded, nil
}

func parseRefreshBenchmarkResult(line string) (refreshBenchmarkResult, error) {
	fields := strings.Fields(line)
	values := make(map[string]string, len(fields))
	for _, field := range fields[1:] {
		key, value, ok := strings.Cut(field, "=")
		if ok {
			values[key] = value
		}
	}
	median, err := strconv.ParseInt(values["median_ns"], 10, 64)
	if err != nil {
		return refreshBenchmarkResult{}, fmt.Errorf("parse benchmark median from %q: %w", line, err)
	}
	p95, err := strconv.ParseInt(values["p95_ns"], 10, 64)
	if err != nil {
		return refreshBenchmarkResult{}, fmt.Errorf("parse benchmark p95 from %q: %w", line, err)
	}
	alloc, err := strconv.ParseUint(values["bytes_per_op"], 10, 64)
	if err != nil {
		return refreshBenchmarkResult{}, fmt.Errorf("parse benchmark allocation from %q: %w", line, err)
	}
	return refreshBenchmarkResult{mode: values["mode"], median: time.Duration(median), p95: time.Duration(p95), alloc: alloc}, nil
}
