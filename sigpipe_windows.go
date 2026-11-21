//go:build windows

package main

// keepRunningOnClosedStdout is a no-op: Windows has no SIGPIPE, and a write
// to a closed pipe already returns an error.
func keepRunningOnClosedStdout() {}
