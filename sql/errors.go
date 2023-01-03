package sql

import (
	"errors"
	"fmt"
)

var errDockerNonZeroExitCodeError = errors.New("docker command has returned a non-zero exit code")

func DockerNonZeroExitCodeError(statusCode int64) error {
	return fmt.Errorf("%w:%d", errDockerNonZeroExitCodeError, statusCode)
}
