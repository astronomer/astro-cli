//go:build darwin

package airflow

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

const scutilPath = "/usr/sbin/scutil"

// hasSystemProxy checks whether macOS has any system-level proxy configured
// by running `scutil --proxy` and looking for any `*Enable : 1` entry.
// Returns false (no proxy) on any error so we default to setting NO_PROXY.
var hasSystemProxy = func() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) //nolint:mnd
	defer cancel()

	out, err := exec.CommandContext(ctx, scutilPath, "--proxy").Output() //nolint:gosec
	if err != nil {
		return false
	}
	// Any "*Enable : 1" line means a proxy is active.  The space-padded
	// format is stable across macOS versions and won't match harmless keys
	// like "FTPPassive : 1" which lack the "Enable" suffix.
	return bytes.Contains(out, []byte("Enable : 1"))
}
