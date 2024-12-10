package runtimes

import "runtime"

type OSChecker interface {
	IsMac() bool
	IsWindows() bool
}

type osChecker struct{}

func CreateOSChecker() OSChecker {
	return new(osChecker)
}

// IsWindows is a utility function to determine if the CLI host machine
// is running on Microsoft Windows OS.
func (o osChecker) IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsMac is a utility function to determine if the CLI host machine
// is running on Apple macOS.
func (o osChecker) IsMac() bool {
	return runtime.GOOS == "darwin"
}
