//go:build !windows

package input

import "syscall"

// stdinFD is the descriptor Password reads from.
//
// Split by platform because the conversion is not the same fact on both:
// syscall.Stdin is already an int here, so writing int() around it is the
// redundant conversion unconvert exists to catch — while on Windows it is a
// Handle and the conversion is required. One expression cannot be both, and
// suppressing it cannot bridge them: a directive needed on Unix reads as a dead
// one under GOOS=windows, which nolintlint then fails the build over.
func stdinFD() int { return syscall.Stdin }
