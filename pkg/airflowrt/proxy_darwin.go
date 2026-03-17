//go:build darwin

package airflowrt

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

const scutilPath = "/usr/sbin/scutil"

// HasSystemProxy checks whether macOS has any system-level proxy configured
// by running `scutil --proxy` and looking for any `*Enable : 1` entry.
// Returns false (no proxy) on any error so we default to setting NO_PROXY.
var HasSystemProxy = func() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) //nolint:mnd
	defer cancel()

	out, err := exec.CommandContext(ctx, scutilPath, "--proxy").Output() //nolint:gosec
	if err != nil {
		return false
	}
	return bytes.Contains(out, []byte("Enable : 1"))
}
