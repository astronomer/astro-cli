package validation

import (
	"fmt"
)

type ValidationError struct {
	Message   string
	Command   string
	Arguments []string
	// TODO Add Flags
	// Flags     []string
}

func (err *ValidationError) Error() string {
	args := ""
	for i, arg := range err.Arguments {
		if i != 0 {
			arg = ", " + arg
		}
		args += arg
	}

	return fmt.Sprintf("ValidationError (%s): %s for args (%v)", err.Command, err.Message, args)
}

// New HoustonError
func New(message, command string, args []string) *ValidationError {
	return &ValidationError{
		Message:   message,
		Command:   command,
		Arguments: args,
		// Flags:     flags,
	}
}
