//go:build !darwin && !linux

package renderer

import "runtime"

func processPeakBytes() (uint64, error) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return memory.Sys, nil
}
