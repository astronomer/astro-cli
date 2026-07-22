//go:build windows

package supervise

import "golang.org/x/sys/windows"

// WaitForParent blocks until the process with the given pid exits. Standalone
// mode does not run on Windows, but docker mode does, and its session-tied
// clean-up watches the starting process here. OpenProcess + WaitForSingleObject
// is event-driven; a pid that has already gone returns at once.
func WaitForParent(pid int) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // pid is a positive process id from the caller
	if err != nil {
		return
	}
	defer func() { _ = windows.CloseHandle(h) }()
	_, _ = windows.WaitForSingleObject(h, windows.INFINITE)
}
