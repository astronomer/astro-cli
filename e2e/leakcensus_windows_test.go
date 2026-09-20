//go:build e2e && windows

package e2e

import "context"

// daemons finds nothing on Windows, where the suite does not start one.
//
// The proxy daemon axis is a pgrep, and the cases that start a daemon are all
// !windows for the same reason. Windows runs tier 0, which starts no runtime at
// all, so there is nothing here for a census to miss — but this is a real gap
// rather than a checked-and-clean answer, and anyone who makes a Windows case
// start a runtime has to write this properly first.
func daemons(_ context.Context) ([]string, error) { return nil, nil }
