package runtimes

import "github.com/astronomer/astro-cli/pkg/fileutil"

// FileChecker interface defines a method to check if a file exists.
// This is here mostly for testing purposes. This allows us to mock
// around actually checking for binaries on a live system as that
// would create inconsistencies across developer machines when
// working with the unit tests.
type FileChecker interface {
	Exists(path string) bool
}

// fileChecker is a concrete implementation of FileChecker.
type fileChecker struct{}

func CreateFileChecker() FileChecker {
	return new(fileChecker)
}

// Exists checks if the file exists in the file system.
func (f fileChecker) Exists(path string) bool {
	exists, _ := fileutil.Exists(path, nil)
	return exists
}
