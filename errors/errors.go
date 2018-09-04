package errors

import (
	"fmt"

	// "github.com/astronomerio/astro-cli/errors/command"
	"github.com/astronomerio/astro-cli/errors/command"
	"github.com/astronomerio/astro-cli/errors/houston"
	"github.com/astronomerio/astro-cli/errors/validation"
	wrappedError "github.com/pkg/errors"
)

var (
	GeneralError    = wrappedError.Wrap
	HoustonError    = houston.New
	CommandError    = command.New
	ValidationError = validation.New
	// ConfigError = config.New
	// CmdErrorHandler handles all errors at a command level. It prevents
	// cobra command usage output if error is decided to be unrelated to
	// a flaw in the cobra command itself, only outputting a failure message instead
	CmdErrorHandler = func(e error) error {
		// If HoustonError, don't return command usage hint
		if he, ok := e.(*houston.HoustonError); ok {
			fmt.Println(he)
			return nil
		} else {
			return e
		}
	}
)
