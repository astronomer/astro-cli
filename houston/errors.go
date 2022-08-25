package houston

import (
	"fmt"
	"strings"
)

type ErrAPINotImplemented struct {
	APIName string
}

func (e ErrAPINotImplemented) Error() string {
	return fmt.Sprintf("%s GraphQL API not implemented for the given Houston version", e.APIName)
}

type ErrFieldsNotAvailable struct {
	BaseError error
}

func (e ErrFieldsNotAvailable) Error() string {
	return "Some fields requested by the CLI are not available in the server schema."
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
	return fmt.Sprintf("no workspaces with id (%s) found", e.workspaceID)
}
