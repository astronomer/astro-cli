package sql

import (
	"errors"
	"fmt"
)

var errArgNotSetError = errors.New("argument not set")

func ArgNotSetError(argument string) error {
	return fmt.Errorf("%w:%s", errArgNotSetError, argument)
}
