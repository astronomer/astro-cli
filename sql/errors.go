package sql

import (
	"errors"
	"fmt"
)

var errEnvVarNotSetError = errors.New("environment Variable not set")

func EnvVarNotSetError(envVar string) error {
	return fmt.Errorf("%w:%s", errEnvVarNotSetError, envVar)
}
