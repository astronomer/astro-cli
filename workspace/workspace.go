package workspace

import (
	"errors"
	"fmt"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/jsonstr"
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
	r := "  %-44s %-50s"
	colorFmt := "\033[33;m"
	colorTrm := "\033[0m"

	ws, err := api.GetWorkspaceAll()
	if err != nil {
		return err
	}

	c, err := config.GetCurrentContext()

	head := fmt.Sprintf(r, "NAME", "UUID")
	fmt.Println(head)
	for _, w := range ws {
		name := w.Label
		workspace := w.Uuid

		fullStr := fmt.Sprintf(r, name, workspace)
		if c.Workspace == w.Uuid {
			fullStr = colorFmt + fullStr + colorTrm
		}

		fmt.Println(fullStr)
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

// GetCurrentWorkspace gets the current workspace set in context config
// Returns a string representing the current workspace and an error if it doesn't exist
func GetCurrentWorkspace() (string, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}

	if len(c.Workspace) == 0 {
		return "", errors.New("Current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACID")
	}

	return c.Workspace, nil
}

// Switch switches workspaces
func Switch(uuid string) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	c.Workspace = uuid
	err = c.SetContext()
	if err != nil {
		return err
	}

	config.PrintCurrentContext()

	return nil
}

// Update an astronomer workspace
func Update(workspaceId string, args map[string]string) error {
	s := jsonstr.MapToJsonObjStr(args)

	ws, err := api.UpdateWorkspace(workspaceId, s)
	if err != nil {
		return err
	}

	fmt.Printf(messages.HOUSTON_WORKSPACE_UPDATE_SUCCESS, ws.Uuid)

	return nil
}
