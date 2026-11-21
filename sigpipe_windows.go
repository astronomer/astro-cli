//go:build windows

package main

// keepRunningOnClosedStdout is a no-op: Windows has no SIGPIPE, and a write
// to a closed pipe already returns an error, which cliout.ClosedStdout reads
// (a direct write to os.Stdout only).
func keepRunningOnClosedStdout() {}
