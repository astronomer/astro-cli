//go:build !windows

package supervise

// WaitForParent blocks until the process with the given pid exits. It exposes
// the same event-driven watch the supervisor uses (kqueue on darwin, pidfd on
// linux, a poll fallback elsewhere) so docker mode can tie a compose project's
// lifetime to a starting session without a child process of its own.
func WaitForParent(pid int) { waitForProcessExit(pid) }
