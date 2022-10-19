package sql

import (
	"errors"
	"fmt"
)

var envVarNotSetError = errors.New("Environment Variable not set")

func EnvVarNotSetError(envVar string) error {
	return fmt.Errorf("%w:%s", envVarNotSetError, envVar)
}
