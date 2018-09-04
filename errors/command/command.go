package command

import "fmt"

type CommandError struct {
	Message string
	Command string
	// Arguments []string
}

func (err *CommandError) Error() string {
	return fmt.Sprintf("CommandError (%s): %s", err.Command, err.Message)
}

// New HoustonError
func New(message, command string) *CommandError {
	return &CommandError{
		Message: message,
		Command: command,
	}
}
