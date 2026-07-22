//go:build !windows

package supervise

import (
	"syscall"
	"time"
)

// pollProcessExit returns when the given pid no longer exists. Fallback for
// when the platform's event-driven mechanism (kqueue on darwin, pidfd on
// linux) isn't available.
func pollProcessExit(pid int) {
	for {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(1 * time.Second)
	}
}
