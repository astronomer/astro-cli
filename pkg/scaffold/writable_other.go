//go:build !unix

package scaffold

import "io/fs"

// writable reports whether this process may write the file at path itself.
//
// On Windows the read-only attribute is what Go reports as a missing owner
// write bit, and it is the attribute a person sets to protect a file, so the
// bit is the check.
func writable(_ string, mode fs.FileMode) bool {
	return mode&0o200 != 0
}
