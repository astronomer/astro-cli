package houston

import (
	"fmt"
	"strings"
)

type ErrAPINotImplemented struct {
	APIName string
}

func (e ErrAPINotImplemented) Error() string {
	return fmt.Sprintf("%s GraphQL API is not implemented in this Houston version", e.APIName)
}

type ErrFieldsNotAvailable struct {
	BaseError error
}

func (e ErrFieldsNotAvailable) Error() string {
	msg := "Some fields the CLI requests are not in the server schema"
	if e.BaseError != nil {
		return msg + ": " + e.BaseError.Error()
	}
	return msg
}

func handleAPIErr(err error) error {
	if strings.Contains(err.Error(), "Cannot query field") {
		return ErrFieldsNotAvailable{
			BaseError: err,
		}
	}

	return err
}

type ErrWorkspaceNotFound struct {
	workspaceID string
}

func (e ErrWorkspaceNotFound) Error() string {
	return fmt.Sprintf("no Workspace found with ID %s", e.workspaceID)
}
