package workspace

import (
	"fmt"

	"github.com/astronomerio/astro-cli/messages"
)

// Add a user to a workspace
func Add(workspaceUuid, email string) error {
	r, err := api.AddWorkspaceUser(workspaceUuid, email)
	if err != nil {
		return err
	}
	fmt.Printf(messages.HOUSTON_WORKSPACE_USER_ADD_SUCCESS, r.Users[0].Username, r.Uuid)
	return nil
}

// Remove a user from a workspace
func Remove(workspaceUuid, email string) error {
	r, err := api.RemoveWorkspaceUser(workspaceUuid, email)
	if err != nil {
		return err
	}
	fmt.Printf(messages.HOUSTON_WORKSPACE_USER_REMOVE_SUCCESS, r.Users[0].Username, r.Uuid)
	return nil
}
