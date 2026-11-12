package input

// SetTerminals makes stdin and stderr count as terminals (or not) for the rest
// of the process, and returns a function that puts the previous answer back. It
// is the seam for tests of the rule itself. Tests that only simulate a person
// typing need nothing: a test binary counts both as terminals unless told
// otherwise, since a test's stdin is a pipe and its stderr a file.
func SetTerminals(stdin, stderr bool) (restore func()) {
	prev := terminals.Swap(&[2]bool{stdin, stderr})
	return func() { terminals.Store(prev) }
}
