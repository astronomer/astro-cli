//go:build !windows

package local

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChildExitCodeOfASignaledChildIs128PlusTheSignal(t *testing.T) {
	var exitErr *exec.ExitError

	err := exec.Command("sh", "-c", "kill -TERM $$").Run()
	assert.True(t, errors.As(err, &exitErr))
	assert.Equal(t, 143, childExitCode(exitErr), "a child killed by SIGTERM is 143, not 255")

	err = exec.Command("sh", "-c", "kill -HUP $$").Run()
	assert.True(t, errors.As(err, &exitErr))
	assert.Equal(t, 129, childExitCode(exitErr))

	err = exec.Command("sh", "-c", "exit 7").Run()
	assert.True(t, errors.As(err, &exitErr))
	assert.Equal(t, 7, childExitCode(exitErr), "a plain exit keeps its code")
}
