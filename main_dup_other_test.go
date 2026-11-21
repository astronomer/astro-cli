//go:build !windows && !linux

package main

import "syscall"

// dup2 makes newfd a copy of oldfd.
func dup2(oldfd, newfd int) error { return syscall.Dup2(oldfd, newfd) }
