package sql

import (
	"errors"
	"fmt"
)

var (
	errArgNotSetError             = errors.New("argument not set")
	errDockerNonZeroExitCodeError = errors.New("docker command has returned a non-zero exit code")
)

func ArgNotSetError(argument string) error {
	return fmt.Errorf("%w:%s", errArgNotSetError, argument)
}

func DockerNonZeroExitCodeError(statusCode int64) error {
	return fmt.Errorf("%w:%d", errDockerNonZeroExitCodeError, statusCode)
}
