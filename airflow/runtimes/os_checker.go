package runtimes

import "runtime"

type OSChecker interface {
	IsMac() bool
	IsWindows() bool
}

type DefaultOSChecker struct{}

// IsWindows is a utility function to determine if the CLI host machine
// is running on Microsoft Windows OS.
func (o DefaultOSChecker) IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsMac is a utility function to determine if the CLI host machine
// is running on Apple macOS.
func (o DefaultOSChecker) IsMac() bool {
	return runtime.GOOS == "darwin"
}
