//go:build windows

package main

import "context"

// watchClosedStdout is a no-op: Windows has no SIGPIPE, and a write to a
// closed pipe already returns an error, which cliout.ClosedStdout reads.
func watchClosedStdout(context.CancelCauseFunc) {}
