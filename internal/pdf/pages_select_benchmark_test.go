package pdf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

type selectBenchResult struct {
	Mode       string `json:"mode"`
	WallNS     int64  `json:"wall_ns"`
	Allocs     uint64 `json:"allocs"`
	Bytes      uint64 `json:"bytes"`
	PeakRSS    int64  `json:"peak_rss_bytes"`
	PrepPasses int    `json:"prep_passes"`
}

type selectBenchFixture struct {
	kind  string
	pages int
	data  []byte
	path  string
}

type selectBenchSample struct{ wall, allocs, bytes, rss int64 }
type selectBenchMeasurement struct {
	samples []selectBenchSample
	passes  int
}

// TestPDFSelectBenchWorker runs one fresh-output measurement in an isolated
// process so getrusage reports a per-run peak RSS.
func TestPDFSelectBenchWorker(t *testing.T) {
	mode := os.Getenv("PDF_SELECT_BENCH_MODE")
	if mode == "" {
		t.Skip("benchmark worker")
	}
	path := os.Getenv("PDF_SELECT_BENCH_INPUT")
	countPages := 0
	if _, err := fmt.Sscanf(os.Getenv("PDF_SELECT_BENCH_PAGES"), "%d", &countPages); err != nil || countPages < 2 {
		t.Fatalf("invalid page count %q", os.Getenv("PDF_SELECT_BENCH_PAGES"))
	}
	pages := fmt.Sprintf("1,%d,%d", countPages/2, countPages)
	passes := 0
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	var output []byte
	var err error
	if mode == "legacy" {
		firstRead, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		count, countErr := pageCount(context.Background(), firstRead, func() { passes++ })
		if countErr != nil {
			t.Fatal(countErr)
		}
		resolved, parseErr := ParsePages(pages, count)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		secondRead, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		output, err = selectPages(context.Background(), secondRead, resolved, func() { passes++ })
	} else if mode == "one-pass" {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		output, _, err = selectPagesBySpec(context.Background(), data, pages, func() { passes++ })
	} else {
		t.Fatalf("invalid benchmark mode %q", mode)
	}
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "pdf-select-bench-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := writeNew(filepath.Join(dir, "selected.pdf"), output); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		t.Fatal(err)
	}
	peakRSS := usage.Maxrss
	if runtime.GOOS != "darwin" {
		peakRSS *= 1024
	}
	result := selectBenchResult{
		Mode: mode, WallNS: elapsed.Nanoseconds(), Allocs: after.Mallocs - before.Mallocs,
		Bytes: after.TotalAlloc - before.TotalAlloc, PeakRSS: peakRSS, PrepPasses: passes,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("PDF_SELECT_RESULT:%s\n", encoded)
}

// TestPDFSelectBenchmarkReport runs one warm-up and five isolated fresh-output
// runs for each fixed fixture and method, then writes a Markdown report.
func TestPDFSelectBenchmarkReport(t *testing.T) {
	if os.Getenv("PDF_SELECT_BENCH_REPORT") != "1" {
		t.Skip("set PDF_SELECT_BENCH_REPORT=1 to run the PDF selection report")
	}
	fixtures := []selectBenchFixture{
		{kind: "text", pages: 50, data: splitTextFixture(50)},
		{kind: "text", pages: 200, data: splitTextFixture(200)},
		{kind: "text", pages: 500, data: splitTextFixture(500)},
		{kind: "image", pages: 50, data: splitImageFixture(50)},
		{kind: "image", pages: 200, data: splitImageFixture(200)},
		{kind: "image", pages: 500, data: splitImageFixture(500)},
	}
	fixtureDir := t.TempDir()
	for i := range fixtures {
		fixtures[i].path = filepath.Join(fixtureDir, fmt.Sprintf("%s-%d.pdf", fixtures[i].kind, fixtures[i].pages))
		if err := os.WriteFile(fixtures[i].path, fixtures[i].data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	reports := make(map[string]map[string]selectBenchMeasurement)
	for _, fixture := range fixtures {
		hash := sha256.Sum256(fixture.data)
		t.Logf("fixture kind=%s pages=%d bytes=%d sha256=%s", fixture.kind, fixture.pages, len(fixture.data), hex.EncodeToString(hash[:]))
		key := fmt.Sprintf("%s-%d", fixture.kind, fixture.pages)
		reports[key] = make(map[string]selectBenchMeasurement)
		for _, mode := range []string{"legacy", "one-pass"} {
			var measurement selectBenchMeasurement
			for run := 0; run < 6; run++ { // warm-up, then five fresh output files/processes.
				cmd := exec.Command(exe, "-test.run=^TestPDFSelectBenchWorker$", "-test.v")
				cmd.Env = append(os.Environ(),
					"PDF_SELECT_BENCH_MODE="+mode,
					"PDF_SELECT_BENCH_INPUT="+fixture.path,
					fmt.Sprintf("PDF_SELECT_BENCH_PAGES=%d", fixture.pages),
				)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("worker %s %s/%d run %d: %v\n%s", mode, fixture.kind, fixture.pages, run, err, out)
				}
				line := ""
				for _, candidate := range strings.Split(string(out), "\n") {
					if strings.HasPrefix(candidate, "PDF_SELECT_RESULT:") {
						line = strings.TrimPrefix(candidate, "PDF_SELECT_RESULT:")
					}
				}
				if line == "" {
					t.Fatalf("worker result missing: %s", out)
				}
				var result selectBenchResult
				if err := json.Unmarshal([]byte(line), &result); err != nil {
					t.Fatal(err)
				}
				if run == 0 {
					continue
				}
				measurement.passes = result.PrepPasses
				measurement.samples = append(measurement.samples, selectBenchSample{wall: result.WallNS, allocs: int64(result.Allocs), bytes: int64(result.Bytes), rss: result.PeakRSS})
			}
			reports[key][mode] = measurement
		}
	}
	report, err := formatPDFSelectBenchmarkReport(fixtures, reports)
	if err != nil {
		t.Fatal(err)
	}
	path := os.Getenv("PDF_SELECT_BENCH_REPORT_OUT")
	if path == "" {
		path = "pdf-select-benchmark.md"
	}
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func formatPDFSelectBenchmarkReport(fixtures []selectBenchFixture, reports map[string]map[string]selectBenchMeasurement) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# pdf-select one-pass benchmark\n\n")
	fmt.Fprintf(&b, "- Go: `%s`\n- OS/architecture: `%s/%s`\n- CPU: `%s`\n- Revision: `%s`\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, cpuName(), gitRevision())
	fmt.Fprintf(&b, "- Method: one warm-up followed by five runs in fresh processes, each writing a new output file. Median wall time, heap allocations, allocated bytes, and per-process peak RSS are reported. Preparation pass counts are instrumented at the pdfcpu preparation entry points.\n\n")
	fmt.Fprintf(&b, "| Fixture | Source size | SHA-256 | Prep passes old/new | Median wall ms old/new | Wall reduction | Median allocs old/new | Median allocated MiB old/new | Median peak RSS MiB old/new |\n|---|---:|---|---:|---:|---:|---:|---:|---:|\n")
	for _, fixture := range fixtures {
		key := fmt.Sprintf("%s-%d", fixture.kind, fixture.pages)
		old, new := reports[key]["legacy"], reports[key]["one-pass"]
		if len(old.samples) != 5 || len(new.samples) != 5 {
			return "", fmt.Errorf("%s has %d/%d measured runs; want five", key, len(old.samples), len(new.samples))
		}
		oldWall, newWall := median(old.samples, func(s selectBenchSample) int64 { return s.wall }), median(new.samples, func(s selectBenchSample) int64 { return s.wall })
		oldAllocs, newAllocs := median(old.samples, func(s selectBenchSample) int64 { return s.allocs }), median(new.samples, func(s selectBenchSample) int64 { return s.allocs })
		oldBytes, newBytes := median(old.samples, func(s selectBenchSample) int64 { return s.bytes }), median(new.samples, func(s selectBenchSample) int64 { return s.bytes })
		oldRSS, newRSS := median(old.samples, func(s selectBenchSample) int64 { return s.rss }), median(new.samples, func(s selectBenchSample) int64 { return s.rss })
		reduction := float64(oldWall-newWall) * 100 / float64(oldWall)
		hash := sha256.Sum256(fixture.data)
		fmt.Fprintf(&b, "| %s, %d pages | %d bytes | `%s` | %d / %d | %.3f / %.3f | %.1f%% | %d / %d | %.2f / %.2f | %.2f / %.2f |\n",
			fixture.kind, fixture.pages, len(fixture.data), hex.EncodeToString(hash[:]), old.passes, new.passes,
			float64(oldWall)/1e6, float64(newWall)/1e6, reduction, oldAllocs, newAllocs,
			float64(oldBytes)/(1<<20), float64(newBytes)/(1<<20), float64(oldRSS)/(1<<20), float64(newRSS)/(1<<20))
		if fixture.pages >= 200 && reduction < 20 {
			return "", fmt.Errorf("%s wall-time reduction %.1f%% is below 20%%", key, reduction)
		}
		if newAllocs > oldAllocs || newBytes > oldBytes {
			return "", fmt.Errorf("%s increased median allocations or allocated bytes", key)
		}
		if newRSS > oldRSS {
			return "", fmt.Errorf("%s increased median peak RSS", key)
		}
		if old.passes != 2 || new.passes != 1 {
			return "", fmt.Errorf("%s preparation passes = %d/%d, want 2/1", key, old.passes, new.passes)
		}
	}
	return b.String(), nil
}

func median(samples []selectBenchSample, value func(selectBenchSample) int64) int64 {
	values := make([]int64, len(samples))
	for i, sample := range samples {
		values[i] = value(sample)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values[len(values)/2]
}

func cpuName() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return runtime.GOARCH
}

func gitRevision() string {
	rootBytes, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "unknown"
	}
	root := strings.TrimSpace(string(rootBytes))
	revCmd := exec.Command("git", "rev-parse", "HEAD")
	revCmd.Dir = root
	out, err := revCmd.Output()
	if err != nil {
		return "unknown"
	}
	revision := strings.TrimSpace(string(out))
	patchCmd := exec.Command("git", "diff", "--binary", "HEAD", "--",
		"internal/cli/cmd_convert.go", "internal/cli/cmd_pdf_select_test.go",
		"internal/pdf/objects.go", "internal/pdf/pages.go",
		"internal/pdf/pages_select_test.go", "internal/pdf/pages_select_benchmark_test.go")
	patchCmd.Dir = root
	patch, err := patchCmd.Output()
	if err != nil {
		return revision + " (working-tree patch unavailable)"
	}
	hash := sha256.Sum256(patch)
	return fmt.Sprintf("%s + working-tree source patch sha256:%x", revision, hash)
}
