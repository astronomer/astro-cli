//go:build !windows
// +build !windows

package ansi

// InitConsole initializes any platform-specific aspect of the terminal.
// This method will run for all except Windows.
func InitConsole() {}
