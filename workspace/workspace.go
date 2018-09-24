package workspace

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/jsonstr"
	"github.com/astronomerio/astro-cli/pkg/printutil"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)

	tab = printutil.Table{
		Padding:      []int{44, 50},
		Header:       []string{"NAME", "UUID"},
		ColorRowCode: [2]string{"\033[33;m", "\033[0m"},
	}
)

// Create a workspace
func Create(label, desc string) error {
	w, err := api.CreateWorkspace(label, desc)
	if err != nil {
		return err
	}

	tab.AddRow([]string{w.Label, w.Uuid}, false)
	tab.SuccessMsg = "\n Successfully created workspace"
	tab.Print()

	return nil
}

// List all workspaces
func List() error {
	ws, err := api.GetWorkspaceAll()
	if err != nil {
		return err
	}

	c, err := config.GetCurrentContext()
	for _, w := range ws {
		name := w.Label
		workspace := w.Uuid

		if c.Workspace == w.Uuid {
			tab.AddRow([]string{name, workspace}, true)
		} else {
			tab.AddRow([]string{name, workspace}, false)
		}
	}

	tab.Print()

	return nil
}

// Delete a workspace by uuid
func Delete(uuid string) error {
	_, err := api.DeleteWorkspace(uuid)
	if err != nil {
		return err
	}

	// TODO remove tab print until houston properly returns attrs on delete
	// tab.AddRow([]string{w.Label, w.Uuid}, false)
	// tab.SuccessMsg = "\n Successfully deleted workspace"
	// tab.Print()
	fmt.Println("\n Successfully deleted workspace")

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
		return "", errors.New("Current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")
	}

	return c.Workspace, nil
}

// Switch switches workspaces
func Switch(uuid string) error {
	// validate workspace
	_, err := api.GetWorkspace(uuid)
	if err != nil {
		return errors.Wrap(err, "workspace uuid is not valid")
	}

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

	w, err := api.UpdateWorkspace(workspaceId, s)
	if err != nil {
		return err
	}

	tab.AddRow([]string{w.Label, w.Uuid}, false)
	tab.SuccessMsg = "\n Successfully updated workspace"
	tab.Print()

	return nil
}
