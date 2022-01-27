package workspace

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var errWorkspaceContextNotSet = errors.New("current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

// Create a workspace
func Create(label, desc string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceCreateRequest,
		Variables: map[string]interface{}{"label": label, "description": desc},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	w := r.Data.CreateWorkspace

	tab := newTableOut()
	tab.AddRow([]string{w.Label, w.ID}, false)
	tab.SuccessMsg = "\n Successfully created workspace"
	tab.Print(out)

	return nil
}

// List all workspaces
func List(client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.WorkspacesGetRequest,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	ws := r.Data.GetWorkspaces

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	tab := newTableOut()
	for i := range ws {
		w := ws[i]
		name := w.Label
		workspace := w.ID

		var color bool

		if c.Workspace == w.ID {
			color = true
		} else {
			color = false
		}
		tab.AddRow([]string{name, workspace}, color)
	}

	tab.Print(out)

	return nil
}

// Delete a workspace by id
func Delete(id string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceDeleteRequest,
		Variables: map[string]interface{}{"workspaceId": id},
	}

	_, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	// TODO remove tab print until houston properly returns attrs on delete
	// tab.AddRow([]string{w.Label, w.Id}, false)
	// tab.SuccessMsg = "\n Successfully deleted workspace"
	// tab.Print()
	fmt.Fprintln(out, "\n Successfully deleted workspace")

	return nil
}

// GetCurrentWorkspace gets the current workspace set in context config
// Returns a string representing the current workspace and an error if it doesn't exist
func GetCurrentWorkspace() (string, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}

	if c.Workspace == "" {
		return "", errWorkspaceContextNotSet
	}

	return c.Workspace, nil
}

func getWorkspaceSelection(client *houston.Client, out io.Writer) (string, error) {
	tab := newTableOut()
	tab.GetUserInput = true

	req := houston.Request{
		Query: houston.WorkspacesGetRequest,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return "", err
	}

	ws := r.Data.GetWorkspaces

	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}

	for i := range ws {
		w := ws[i]
		name := w.Label
		workspace := w.ID

		var color bool

		if c.Workspace == w.ID {
			color = true
		} else {
			color = false
		}
		tab.AddRow([]string{name, workspace}, color)
	}

	tab.Print(out)

	in := input.Text("\n> ")
	i, err := strconv.ParseInt(in, 10, 64)
	if err != nil {
		return "", fmt.Errorf("cannot parse %s to int: %w", in, err)
	}

	return ws[i-1].ID, nil
}

// Switch switches workspaces
func Switch(id string, client *houston.Client, out io.Writer) error {
	if id == "" {
		_id, err := getWorkspaceSelection(client, out)
		if err != nil {
			return err
		}

		id = _id
	}
	// validate workspace
	req := houston.Request{
		Query:     houston.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceId": id},
	}

	_, err := req.DoWithClient(client)
	if err != nil {
		return fmt.Errorf("workspace id is not valid: %w", err)
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	c.Workspace = id
	err = c.SetContext()
	if err != nil {
		return err
	}

	err = config.PrintCurrentContext(out)
	return err
}

// Update an astronomer workspace
func Update(id string, client *houston.Client, out io.Writer, args map[string]string) error {
	// validate workspace
	req := houston.Request{
		Query:     houston.WorkspaceUpdateRequest,
		Variables: map[string]interface{}{"workspaceId": id, "payload": args},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	w := r.Data.UpdateWorkspace
	tab := newTableOut()
	tab.AddRow([]string{w.Label, w.ID}, false)
	tab.SuccessMsg = "\n Successfully updated workspace"
	tab.Print(out)

	return nil
}
