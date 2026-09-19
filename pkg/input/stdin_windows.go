//go:build windows

package input

import "syscall"

// stdinFD is the descriptor Password reads from. syscall.Stdin is a Handle on
// Windows, so unlike the Unix side this is a real conversion. See stdin_unix.go
// for why the two are separate files.
func stdinFD() int { return int(syscall.Stdin) }
