//go:build linux

package main

import "syscall"

// dup2 makes newfd a copy of oldfd. arm64 Linux has no dup2, only dup3.
func dup2(oldfd, newfd int) error { return syscall.Dup3(oldfd, newfd, 0) }
