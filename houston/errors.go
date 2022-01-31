package houston

import "fmt"

type ErrWorkspaceNotFound struct {
	workspaceID string
}

func (e ErrWorkspaceNotFound) Error() string {
	return fmt.Sprintf("no workspaces with id (%s) found", e.workspaceID)
}
