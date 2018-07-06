package workspace

import (
	"fmt"

	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/httputil"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

// Create a workspace
func Create(label, desc string) error {
	w, err := api.CreateWorkspace(label, desc)
	if err != nil {
		return err
	}
	fmt.Printf(messages.HOUSTON_WORKSPACE_CREATE_SUCCESS, w.Label, w.Uuid)
	return nil
}

// List all workspaces
func List() error {
	ws, err := api.GetWorkspaceAll()
	if err != nil {
		return err
	}

	for _, w := range ws {
		rowTmp := "Label: %s\nId: %s\nDesc.: %s\n\n"

		fmt.Printf(rowTmp, w.Label, w.Uuid, w.Description)
	}
	return nil
}

// Delete a workspace by uuid
func Delete(uuid string) error {
	ws, err := api.DeleteWorkspace(uuid)
	if err != nil {
		return err
	}

	fmt.Printf(messages.HOUSTON_WORKSPACE_DELETE_SUCCESS, ws.Label, ws.Uuid)
	return nil
}
