//go:build darwin

package pdf

import "syscall"

func benchmarkPeakRSSBytes() float64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	// Darwin reports ru_maxrss in bytes.
	return float64(usage.Maxrss)
}
