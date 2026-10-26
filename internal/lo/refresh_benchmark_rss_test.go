//go:build darwin || linux

package lo

import (
	"os"
	"runtime"
	"syscall"
)

func processPeakRSSBytes(state *os.ProcessState) uint64 {
	if state == nil {
		return 0
	}
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok || usage.Maxrss <= 0 {
		return 0
	}
	maxRSS := uint64(usage.Maxrss)
	if runtime.GOOS == "linux" {
		maxRSS *= 1024
	}
	return maxRSS
}
