package cmd

import (
	"errors"
)

var (
	errInvalidSetArgs    = errors.New("must specify exactly two arguments (key value) when setting a config")
	errInvalidConfigPath = errors.New("unknown setting")
)
