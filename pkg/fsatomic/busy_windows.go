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

// isBusy reports whether an operation failed because someone else holds the
// file open.
//
// ERROR_SHARING_VIOLATION means exactly that. ERROR_ACCESS_DENIED does not:
// Windows also returns it for a read-only file, an ACL that denies the caller,
// and a destination that is a directory. It is kept here anyway, because a
// rename onto an open file reports it often enough that dropping it would put
// the contention this package exists for back on the floor — and contention is
// the overwhelmingly common cause for the files it serves, which are state two
// processes write by design.
//
// The cost is that a genuine permission failure is retried for the whole budget
// before being reported, with the right error but two seconds late. That is the
// wrong trade only if permission failures are common here, and they are not:
// these paths are under the user's own ~/.astro. Narrowing it — error 5 treated
// as busy only when the destination exists and is a regular file — would need a
// stat between the attempts and is worth doing if anyone ever reports the wait.
func isBusy(err error) bool {
	return errors.Is(err, errorAccessDenied) || errors.Is(err, errorSharingViolation)
}
