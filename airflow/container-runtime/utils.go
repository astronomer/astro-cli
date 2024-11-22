package container_runtime

import "runtime"

// isWindows is a utility function to determine if the CLI host machine
// is running on Microsoft Windows OS.
func isWindows() bool {
	return runtime.GOOS == "windows"
}

// isMac is a utility function to determine if the CLI host machine
// is running on Apple macOS.
func isMac() bool {
	return runtime.GOOS == "darwin"
}
