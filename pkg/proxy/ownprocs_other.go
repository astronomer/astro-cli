//go:build !darwin && !linux && !windows

package proxy

// ownProcesses finds nothing where there is no reader for it, so the daemon
// never stops a process there and falls back to another port instead.
func ownProcesses() []process { return nil }
