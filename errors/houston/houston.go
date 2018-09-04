package houston

import "fmt"

type HoustonError struct {
	Message    string
	StatusCode int
	// Arguments []string
}

func (err *HoustonError) Error() string {
	return fmt.Sprintf("HoustonError (%d): %s", err.StatusCode, err.Message)
}

// New HoustonError
func New(message string, code int) *HoustonError {
	return &HoustonError{
		Message:    message,
		StatusCode: code,
	}
}
