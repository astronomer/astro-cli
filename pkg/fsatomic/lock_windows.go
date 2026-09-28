//go:build windows

package fsatomic

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// LockFileEx and UnlockFileEx, loaded from kernel32 rather than taken from
// golang.org/x/sys/windows: this module has no dependencies, and busy_windows.go
// already declined to add one for a constant. Package syscall exports the
// loader and the OVERLAPPED struct; it does not wrap these two calls.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// Win32 values, fixed parts of the API.
const (
	lockfileFailImmediately = 0x1
	lockfileExclusiveLock   = 0x2
	// errorLockViolation is what a FAIL_IMMEDIATELY attempt returns while
	// another handle holds the range.
	errorLockViolation = syscall.Errno(33) // ERROR_LOCK_VIOLATION
	// The whole file, the conventional range. The lock file holds no data;
	// what matters is that every holder asks for the same bytes.
	lockRangeLow  = 0xFFFFFFFF
	lockRangeHigh = 0xFFFFFFFF
)

// tryLock makes one non-blocking attempt at an exclusive LockFileEx. held is
// false, with no error, when another handle holds it.
func tryLock(f *os.File) (held bool, err error) {
	var ol syscall.Overlapped
	r, _, callErr := procLockFileEx.Call(
		f.Fd(),
		lockfileExclusiveLock|lockfileFailImmediately,
		0,
		lockRangeLow, lockRangeHigh,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r != 0 {
		return true, nil
	}
	if errors.Is(callErr, errorLockViolation) {
		return false, nil
	}
	return false, callErr
}

// unlockFile releases the lock. Closing the handle releases it too, so a
// failure here changes nothing a caller could act on.
func unlockFile(f *os.File) {
	var ol syscall.Overlapped
	procUnlockFileEx.Call(f.Fd(), 0, lockRangeLow, lockRangeHigh, uintptr(unsafe.Pointer(&ol))) //nolint:errcheck // see above
}
