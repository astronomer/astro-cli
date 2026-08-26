//go:build windows

package fsatomic

import (
	"errors"
	"syscall"
)

// The two Win32 codes a rename returns when the destination is held open by
// another handle. Spelled as numbers because package syscall does not export a
// name for the second, and pulling golang.org/x/sys into a module that has no
// dependencies at all is a poor trade for one constant. The values are fixed
// parts of the Win32 API.
const (
	errorAccessDenied     = syscall.Errno(5)  // ERROR_ACCESS_DENIED
	errorSharingViolation = syscall.Errno(32) // ERROR_SHARING_VIOLATION
)

// isBusy reports whether a rename failed because someone else holds the
// destination open. Both are transient: another write or read in flight, not a
// permission problem.
func isBusy(err error) bool {
	return errors.Is(err, errorAccessDenied) || errors.Is(err, errorSharingViolation)
}
