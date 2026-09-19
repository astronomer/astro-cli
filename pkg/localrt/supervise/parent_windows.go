//go:build windows

package supervise

import "golang.org/x/sys/windows"

// WaitForParent blocks until the process with the given pid exits. Standalone
// mode does not run on Windows, but docker mode does, and its session-tied
// clean-up watches the starting process here. OpenProcess + WaitForSingleObject
// is event-driven; a pid that has already gone returns at once.
func WaitForParent(pid int) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	// Both returns are dropped on purpose. This function reports nothing — the
	// wait is the whole contract — and a handle that will not close or a wait
	// that ends early leaves the caller doing exactly what it would anyway.
	defer func() { _ = windows.CloseHandle(h) }()           //nolint:errcheck // deliberate, for the reason above
	_, _ = windows.WaitForSingleObject(h, windows.INFINITE) //nolint:errcheck // deliberate, for the reason above
}
