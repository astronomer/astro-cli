package ansi

import (
	"golang.org/x/sys/windows"
)

// InitConsole configures the standard output and error streams
// on Windows systems. This is necessary to enable colored and ANSI output.
// This is the Windows implementation of ansi/init.go.
func InitConsole() {
	setWindowsConsoleMode(windows.Stdout, windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	setWindowsConsoleMode(windows.Stderr, windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}

func setWindowsConsoleMode(handle windows.Handle, flags uint32) {
	var mode uint32
	// set the console mode if not already there:
	if err := windows.GetConsoleMode(handle, &mode); err == nil {
		// A console that will not take the mode prints escape codes instead of
		// color, which is ugly and not worth failing anything over.
		_ = windows.SetConsoleMode(handle, mode|flags) //nolint:errcheck // deliberate, for the reason above
	}
}
