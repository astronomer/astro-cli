//go:build unix

package scaffold

import (
	"io/fs"
	"syscall"
)

// accessWriteOK is access(2)'s W_OK. Spelled as a number because package
// syscall does not export it on every Unix, and the value is fixed by POSIX.
const accessWriteOK = 0x2

// writable reports whether this process may write the file at path itself.
//
// Asked of the kernel rather than read from the permission bits: the owner's
// write bit says nothing about a file someone else owns, and the atomic rename
// that replaces the manifest needs only the directory to be writable, so a
// check of the bits alone would let a user replace a file they could not
// write, and take its ownership in the process.
func writable(path string, _ fs.FileMode) bool {
	return syscall.Access(path, accessWriteOK) == nil
}
