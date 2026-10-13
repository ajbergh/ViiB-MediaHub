//go:build !windows

package api

// Unsupported memory instrumentation is absent, never reported as zero RSS.
func preparationPeakWorkingSet() *uint64 { return nil }
