//go:build !darwin && !linux && !windows

package supervise

// waitForProcessExit polls the given pid and returns when the process no
// longer exists. Used on platforms without an event-driven mechanism.
func waitForProcessExit(pid int) {
	pollProcessExit(pid)
}
