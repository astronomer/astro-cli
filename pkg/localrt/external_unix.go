//go:build !windows

package localrt

// claimSupported reports whether a standalone claim can work on this platform.
// It can here: liveness is a real process-group signal.
func claimSupported() error { return nil }
