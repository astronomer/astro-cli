package mocks

// FileChecker is a mock implementation of FileChecker for tests.
// This is a manually created mock, not generated by mockery.
type FileChecker struct {
	ExistingFiles map[string]bool
}

// Exists is just a mock for os.Stat(). In our test implementation, we just check
// if the file exists in the list of mocked files for a given test.
func (m FileChecker) Exists(path string) bool {
	return m.ExistingFiles[path]
}
