//go:build !windows

package auth

import (
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestQuitOnInterruptExitsOnTheFirstInterrupt(t *testing.T) {
	original := exitOnInterrupt
	t.Cleanup(func() { exitOnInterrupt = original })

	exited := make(chan int, 1)
	exitOnInterrupt = func(code int) { exited <- code }
	stop := quitOnInterrupt()
	defer stop()

	assert.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))

	select {
	case code := <-exited:
		assert.Equal(t, 130, code)
	case <-time.After(5 * time.Second):
		t.Fatal("the interrupt did not end the login")
	}
}
