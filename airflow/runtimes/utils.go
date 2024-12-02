package runtimes

import "runtime"

// IsWindows is a utility function to determine if the CLI host machine
// is running on Microsoft Windows OS.
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// isMac is a utility function to determine if the CLI host machine
// is running on Apple macOS.
func isMac() bool {
	return runtime.GOOS == "darwin"
}
