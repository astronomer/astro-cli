//go:build !windows

package fsatomic

// isBusy is always false here: rename(2) is atomic and does not fail because
// another process has the destination open, so there is nothing to wait for.
func isBusy(error) bool { return false }
