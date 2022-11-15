package workspace

import (
	"io"
	"strconv"

	"github.com/pkg/errors"

	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var errInvalidWorkspaceKey = errors.New("invalid workspace selection")

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

// GetCurrentWorkspace gets the current workspace set in context config
// Returns a string representing the current workspace and an error if it doesn't exist
func GetCurrentWorkspace() (string, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}

	if c.Workspace == "" {
		return "", errors.New("current workspace context not set, you can switch to a workspace with \n\astro workspace switch WORKSPACEID")
	}

	return c.Workspace, nil
}

// List all workspaces
func List(client astro.Client, out io.Writer) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	ws, err := client.ListWorkspaces(c.Organization)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	tab := newTableOut()
	for i := range ws {
		name := ws[i].Label
		workspace := ws[i].ID

		var color bool

		if c.Workspace == ws[i].ID {
			color = true
		} else {
			color = false
		}
		tab.AddRow([]string{name, workspace}, color)
	}

	tab.Print(out)

	return nil
}

func getWorkspaceSelection(client astro.Client, out io.Writer) (string, error) {
	tab := printutil.Table{
		Padding:        []int{5, 44, 50},
		DynamicPadding: true,
		Header:         []string{"#", "NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}

	var c config.Context
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}

	ws, err := client.ListWorkspaces(c.Organization)
	if err != nil {
		return "", err
	}

	deployMap := map[string]astro.Workspace{}
	for i := range ws {
		index := i + 1

		color := c.Workspace == ws[i].ID
		tab.AddRow([]string{strconv.Itoa(index), ws[i].Label, ws[i].ID}, color)

		deployMap[strconv.Itoa(index)] = ws[i]
	}
	tab.Print(out)
	choice := input.Text("\n> ")
	selected, ok := deployMap[choice]
	if !ok {
		return "", errInvalidWorkspaceKey
	}

	return selected.ID, nil
}

// Switch switches workspaces
func Switch(id string, client astro.Client, out io.Writer) error {
	if id == "" {
		_id, err := getWorkspaceSelection(client, out)
		if err != nil {
			return err
		}

		id = _id
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	// validate workspace
	_, err = client.ListWorkspaces(c.Organization)
	if err != nil {
		return errors.Wrap(err, "workspace id is not valid")
	}

	err = c.SetContextKey("workspace", id)
	if err != nil {
		return err
	}

	err = c.SetContextKey("last_used_workspace", id)
	if err != nil {
		return err
	}

	err = c.SetContextKey("organization", c.Organization)
	if err != nil {
		return err
	}

	err = c.SetContextKey("organization_short_name", c.OrganizationShortName)
	if err != nil {
		return err
	}

	err = config.PrintCurrentCloudContext(out)
	if err != nil {
		return err
	}

	return nil
}
