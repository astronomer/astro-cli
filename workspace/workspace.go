package workspace

import (
	"fmt"
	"io"
	"strconv"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/astrohub"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

// Create a workspace
func Create(label, desc string, client *astrohub.Client, out io.Writer) error {
	req := astrohub.Request{
		Query:     astrohub.WorkspaceCreateRequest,
		Variables: map[string]interface{}{"label": label, "description": desc},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	w := r.Data.CreateWorkspace

	tab := newTableOut()
	tab.AddRow([]string{w.Label, w.Id}, false)
	tab.SuccessMsg = "\n Successfully created workspace"
	tab.Print(out)

	return nil
}

// List all workspaces
func List(client *astrohub.Client, out io.Writer) error {
	req := astrohub.Request{
		Query: astrohub.WorkspacesGetRequest,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	ws := r.Data.GetWorkspaces

	c, err := config.GetCurrentContext()
	tab := newTableOut()
	for _, w := range ws {
		name := w.Label
		workspace := w.Id

		var color bool

		if c.Workspace == w.Id {
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
func Delete(id string, client *astrohub.Client, out io.Writer) error {
	req := astrohub.Request{
		Query:     astrohub.WorkspaceDeleteRequest,
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

	if len(c.Workspace) == 0 {
		return "", errors.New("Current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")
	}

	return c.Workspace, nil
}

func getWorkspaceSelection(client *astrohub.Client, out io.Writer) (string, error) {
	tab := newTableOut()
	tab.GetUserInput = true

	req := astrohub.Request{
		Query: astrohub.WorkspacesGetRequest,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return "", err
	}

	ws := r.Data.GetWorkspaces

	c, err := config.GetCurrentContext()

	for _, w := range ws {
		name := w.Label
		workspace := w.Id

		var color bool

		if c.Workspace == w.Id {
			color = true
		} else {
			color = false
		}
		tab.AddRow([]string{name, workspace}, color)
	}

	tab.Print(out)

	in := input.InputText("\n> ")
	i, err := strconv.ParseInt(
		in,
		10,
		64,
	)

	if err != nil {
		return "", errors.Wrapf(err, "cannot parse %s to int", in)
	}

	return ws[i-1].Id, nil
}

// Switch switches workspaces
func Switch(id string, client *astrohub.Client, out io.Writer) error {
	if len(id) == 0 {
		_id, err := getWorkspaceSelection(client, out)
		if err != nil {
			return err
		}

		id = _id
	}
	// validate workspace
	req := astrohub.Request{
		Query:     astrohub.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceId": id},
	}

	_, err := req.DoWithClient(client)
	if err != nil {
		return errors.Wrap(err, "workspace id is not valid")
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

	config.PrintCurrentContext(out)

	return nil
}

// Update an astronomer workspace
func Update(id string, client *astrohub.Client, out io.Writer, args map[string]string) error {
	// validate workspace
	req := astrohub.Request{
		Query:     astrohub.WorkspaceUpdateRequest,
		Variables: map[string]interface{}{"workspaceId": id, "payload": args},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	w := r.Data.UpdateWorkspace
	tab := newTableOut()
	tab.AddRow([]string{w.Label, w.Id}, false)
	tab.SuccessMsg = "\n Successfully updated workspace"
	tab.Print(out)

	return nil
}
