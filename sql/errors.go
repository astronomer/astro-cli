package sql

import (
	"errors"
	"fmt"
)

var errEnvVarNotSetError = errors.New("environment Variable not set")
var errArgNotSetError = errors.New("argument not set")

func EnvVarNotSetError(envVar string) error {
	return fmt.Errorf("%w:%s", errEnvVarNotSetError, envVar)
}

func ArgNotSetError(argument string) error {
	return fmt.Errorf("%w:%s", errArgNotSetError, argument)
}
