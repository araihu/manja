//go:build darwin

package renderer

import "syscall"

func processPeakBytes() (uint64, error) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, err
	}
	if usage.Maxrss < 0 {
		return 0, nil
	}
	return uint64(usage.Maxrss), nil
}
