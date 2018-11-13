package workspace

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/input"
	"github.com/astronomerio/astro-cli/pkg/printutil"
)

var (
	tab = printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "UUID"},
		ColorRowCode:   [2]string{"\033[33;m", "\033[0m"},
	}
)

// Create a workspace
func Create(label, desc string) error {
	req := houston.Request{
		Query:     houston.WorkspaceCreateRequest,
		Variables: map[string]interface{}{"label": label, "description": desc},
	}

	r, err := req.Do()
	if err != nil {
		return err
	}

	w := r.Data.CreateWorkspace

	tab.AddRow([]string{w.Label, w.Uuid}, false)
	tab.SuccessMsg = "\n Successfully created workspace"
	tab.Print()

	return nil
}

// List all workspaces
func List() error {
	req := houston.Request{
		Query: houston.WorkspacesGetRequest,
	}

	r, err := req.Do()
	if err != nil {
		return err
	}

	ws := r.Data.GetWorkspaces

	c, err := config.GetCurrentContext()

	rows := []printutil.TempRow{}

	for _, w := range ws {
		name := w.Label
		workspace := w.Uuid

		var color bool

		if c.Workspace == w.Uuid {
			color = true
		} else {
			color = false
		}
		row := printutil.TempRow{Values: []string{name, workspace}, Color: color}
		rows = append(rows, row)
	}

	tab.AddRows(rows)
	tab.Print()

	return nil
}

// Delete a workspace by uuid
func Delete(uuid string) error {
	req := houston.Request{
		Query:     houston.WorkspaceDeleteRequest,
		Variables: map[string]interface{}{"workspaceUuid": uuid},
	}

	_, err := req.Do()
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

func getWorkspaceSelection() (string, error) {
	tab.GetUserInput = true

	req := houston.Request{
		Query: houston.WorkspacesGetRequest,
	}

	r, err := req.Do()
	if err != nil {
		return "", err
	}

	ws := r.Data.GetWorkspaces

	c, err := config.GetCurrentContext()

	rows := []printutil.TempRow{}

	for _, w := range ws {
		name := w.Label
		workspace := w.Uuid

		var color bool

		if c.Workspace == w.Uuid {
			color = true
		} else {
			color = false
		}
		row := printutil.TempRow{Values: []string{name, workspace}, Color: color}
		rows = append(rows, row)
	}

	tab.AddRows(rows)

	tab.Print()

	in := input.InputText("\n> ")
	i, err := strconv.ParseInt(
		in,
		10,
		64,
	)

	if err != nil {
		return "", errors.Wrapf(err, "cannot parse %s to int", in)
	}

	return ws[i-1].Uuid, nil
}

// Switch switches workspaces
func Switch(uuid string) error {
	if len(uuid) == 0 {
		_uuid, err := getWorkspaceSelection()
		if err != nil {
			return err
		}

		uuid = _uuid
	}
	// validate workspace
	req := houston.Request{
		Query:     houston.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceUuid": uuid},
	}

	_, err := req.Do()
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
func Update(uuid string, args map[string]string) error {
	// validate workspace
	req := houston.Request{
		Query:     houston.WorkspaceUpdateRequest,
		Variables: map[string]interface{}{"workspaceUuid": uuid, "payload": args},
	}

	r, err := req.Do()
	if err != nil {
		return err
	}

	w := r.Data.UpdateWorkspace

	tab.AddRow([]string{w.Label, w.Uuid}, false)
	tab.SuccessMsg = "\n Successfully updated workspace"
	tab.Print()

	return nil
}
