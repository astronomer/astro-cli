//go:build windows

package proxy

// IsPIDAlive on Windows always returns false.
// The proxy daemon is not supported on Windows.
var IsPIDAlive = func(pid int) bool {
	return false
}
