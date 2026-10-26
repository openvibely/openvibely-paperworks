//go:build !darwin && !linux

package lo

import "os"

func processPeakRSSBytes(*os.ProcessState) uint64 { return 0 }
