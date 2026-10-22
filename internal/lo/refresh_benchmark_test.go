package lo

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/openvibely/openvibely-paperworks/internal/testdeps"
	"github.com/xuri/excelize/v2"
)

// TestRefreshBenchmarkSample is an opt-in acceptance benchmark. Each operation
// uses one throwaway LibreOffice profile when the workbook needs Office; safe
// formula-free workbooks use the local cache refresh path.
// Set PAPERWORKS_REFRESH_BENCHMARK_GROUP to small-cold, small-warm, large-cold,
// or large-warm to run one 30-operation sample.
func TestRefreshBenchmarkSample(t *testing.T) {
	group := os.Getenv("PAPERWORKS_REFRESH_BENCHMARK_GROUP")
	if group == "" {
		t.Skip("set PAPERWORKS_REFRESH_BENCHMARK_GROUP to run the refresh benchmark")
	}
	if err := testdeps.Office(Find); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(group, "-")
	if len(parts) != 2 || (parts[0] != "small" && parts[0] != "large") || (parts[1] != "cold" && parts[1] != "warm") {
		t.Fatalf("invalid benchmark group %q", group)
	}
	input := "../../testdata/fixtures/reference-pie.xlsx"
	if parts[0] == "large" {
		input = filepath.Join(os.TempDir(), "paperworks-refresh-benchmark-large.xlsx")
		if _, err := os.Stat(input); os.IsNotExist(err) {
			createRefreshBenchmarkWorkbook(t, input)
		} else if err != nil {
			t.Fatal(err)
		}
	}
	inputInfo, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	ctx := context.Background()
	var launches int
	runner := commandRunner(func(cmd *exec.Cmd) ([]byte, error) {
		launches++
		return cmd.CombinedOutput()
	})
	ctx = context.WithValue(ctx, commandRunnerKey{}, runner)
	run := func(i int) time.Duration {
		t.Helper()
		out := filepath.Join(root, fmt.Sprintf("%s-%03d.xlsx", group, i))
		started := time.Now()
		if err := Refresh(ctx, input, out, false); err != nil {
			t.Fatalf("refresh %d: %v", i, err)
		}
		elapsed := time.Since(started)
		if info, err := os.Stat(out); err != nil || info.Size() == 0 {
			t.Fatalf("refresh %d output missing or empty: %v", i, err)
		}
		return elapsed
	}
	if parts[1] == "warm" {
		for i := 0; i < 3; i++ {
			run(-i - 1)
		}
		launches = 0
	}
	const samples = 30
	times := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		times = append(times, run(i))
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	median := (times[14] + times[15]) / 2
	p95 := times[int(math.Ceil(.95*float64(samples)))-1]
	if launches%samples != 0 {
		t.Fatalf("launch count %d not divisible by %d measured operations", launches, samples)
	}
	t.Logf("group=%s runs=%d workbook_bytes=%d median=%s p95=%s launches_per_operation=%d", group, samples, inputInfo.Size(), median, p95, launches/samples)
}

func createRefreshBenchmarkWorkbook(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	book := excelize.NewFile()
	defer book.Close()
	for row := 1; row <= 50000; row++ {
		if err := book.SetCellValue("Sheet1", fmt.Sprintf("A%d", row), row); err != nil {
			t.Fatal(err)
		}
		if err := book.SetCellFormula("Sheet1", fmt.Sprintf("B%d", row), fmt.Sprintf("A%d*1.23", row)); err != nil {
			t.Fatal(err)
		}
		if err := book.SetCellValue("Sheet1", fmt.Sprintf("C%d", row), fmt.Sprintf("Record %05d", row)); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
}
