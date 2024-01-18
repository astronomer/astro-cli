package workspace

import (
	httpContext "context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/pkg/errors"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	errInvalidWorkspaceKey = errors.New("invalid workspace selection")
	ErrInvalidName         = errors.New("no name provided for the workspace. Retry with a valid name")
	ErrInvalidTokenName    = errors.New("no name provided for the workspace token. Retry with a valid name")
	ErrWorkspaceNotFound   = errors.New("no workspace was found for the ID you provided")
	ErrNoWorkspaceExists   = errors.New("no workspace was found in your organization")
	ErrWrongEnforceInput   = errors.New("the input to the `--enforce-cicd` flag")
)

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
func List(client astrocore.CoreClient, out io.Writer) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	ws, err := GetWorkspaces(client)
	if err != nil {
		return err
	}

	tab := newTableOut()
	for i := range ws {
		name := ws[i].Name
		workspace := ws[i].Id

		var color bool

		if c.Workspace == ws[i].Id {
			color = true
		} else {
			color = false
		}
		tab.AddRow([]string{name, workspace}, color)
	}

	tab.Print(out)

	return nil
}

var GetWorkspaceSelection = func(client astrocore.CoreClient, out io.Writer) (string, error) {
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

	ws, err := GetWorkspaces(client)
	if err != nil {
		return "", err
	}

	deployMap := map[string]astrocore.Workspace{}
	for i := range ws {
		index := i + 1

		color := c.Workspace == ws[i].Id
		tab.AddRow([]string{strconv.Itoa(index), ws[i].Name, ws[i].Id}, color)

		deployMap[strconv.Itoa(index)] = ws[i]
	}
	tab.Print(out)
	choice := input.Text("\n> ")
	selected, ok := deployMap[choice]
	if !ok {
		return "", errInvalidWorkspaceKey
	}

	return selected.Id, nil
}

func Switch(id string, client astrocore.CoreClient, out io.Writer) error {
	if id == "" {
		_id, err := GetWorkspaceSelection(client, out)
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
	_, err = GetWorkspaces(client)
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

	err = c.SetOrganizationContext(c.Organization, c.OrganizationShortName, c.OrganizationProduct)
	if err != nil {
		return err
	}

	err = config.PrintCurrentCloudContext(out)
	if err != nil {
		return err
	}

	return nil
}

func validateEnforceCD(enforceCD string) (bool, error) {
	var enforce bool
	switch {
	case enforceCD == "OFF" || enforceCD == "":
		enforce = false
	case enforceCD == "ON":
		enforce = true
	default:
		return false, ErrWrongEnforceInput
	}
	return enforce, nil
}

func Create(name, description, enforceCD string, out io.Writer, client astrocore.CoreClient) error {
	if name == "" {
		return ErrInvalidName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	enforce, err := validateEnforceCD(enforceCD)
	if err != nil {
		return err
	}
	workspaceCreateRequest := astrocore.CreateWorkspaceJSONRequestBody{
		ApiKeyOnlyDeploymentsDefault: &enforce,
		Description:                  &description,
		Name:                         name,
	}
	resp, err := client.CreateWorkspaceWithResponse(httpContext.Background(), ctx.Organization, workspaceCreateRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace %s was successfully created\n", name)
	return nil
}

func Update(id, name, description, enforceCD string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	workspaces, err := GetWorkspaces(client)
	if err != nil {
		return err
	}
	var workspace astrocore.Workspace
	if id == "" {
		workspace, err = selectWorkspace(workspaces)
		if workspace.Id == "" {
			return ErrNoWorkspaceExists
		}
		if err != nil {
			return err
		}
	} else {
		for i := range workspaces {
			if workspaces[i].Id == id {
				workspace = workspaces[i]
			}
		}
		if workspace.Id == "" {
			return ErrWorkspaceNotFound
		}
	}
	workspaceID := workspace.Id

	workspaceUpdateRequest := astrocore.UpdateWorkspaceRequest{}

	if name == "" {
		workspaceUpdateRequest.Name = workspace.Name
	} else {
		workspaceUpdateRequest.Name = name
	}

	if description == "" {
		workspaceUpdateRequest.Description = workspace.Description
	} else {
		workspaceUpdateRequest.Description = &description
	}
	if enforceCD == "" {
		workspaceUpdateRequest.ApiKeyOnlyDeploymentsDefault = workspace.ApiKeyOnlyDeploymentsDefault
	} else {
		enforce, err := validateEnforceCD(enforceCD)
		if err != nil {
			return err
		}
		workspaceUpdateRequest.ApiKeyOnlyDeploymentsDefault = enforce
	}
	resp, err := client.UpdateWorkspaceWithResponse(httpContext.Background(), ctx.Organization, workspaceID, workspaceUpdateRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace %s was successfully updated\n", workspace.Name)
	return nil
}

func Delete(id string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	workspaces, err := GetWorkspaces(client)
	if err != nil {
		return err
	}
	var workspace astrocore.Workspace
	if id == "" {
		workspace, err = selectWorkspace(workspaces)
		if workspace.Id == "" {
			return ErrNoWorkspaceExists
		}
		if err != nil {
			return err
		}
	} else {
		for i := range workspaces {
			if workspaces[i].Id == id {
				workspace = workspaces[i]
			}
		}
		if workspace.Id == "" {
			return ErrWorkspaceNotFound
		}
	}
	workspaceID := workspace.Id
	resp, err := client.DeleteWorkspaceWithResponse(httpContext.Background(), ctx.Organization, workspaceID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace %s was successfully deleted\n", workspace.Name)
	return nil
}

func selectWorkspace(workspaces []astrocore.Workspace) (astrocore.Workspace, error) {
	if len(workspaces) == 0 {
		return astrocore.Workspace{}, nil
	}

	if len(workspaces) == 1 {
		fmt.Println("Only one Workspace was found. Using the following Workspace by default: \n" +
			fmt.Sprintf("\n Workspace Name: %s", ansi.Bold(workspaces[0].Name)) +
			fmt.Sprintf("\n Workspace ID: %s\n", ansi.Bold(workspaces[0].Id)))

		return workspaces[0], nil
	}

	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"#", "WORKSPACENAME", "ID", "CICD ENFORCEMENT"},
	}

	fmt.Println("\nPlease select the workspace you would like to update:")

	workspaceMap := map[string]astrocore.Workspace{}
	for i := range workspaces {
		index := i + 1
		table.AddRow([]string{
			strconv.Itoa(index),
			workspaces[i].Name,
			workspaces[i].Id,
			strconv.FormatBool(workspaces[i].ApiKeyOnlyDeploymentsDefault),
		}, false)
		workspaceMap[strconv.Itoa(index)] = workspaces[i]
	}

	table.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := workspaceMap[choice]
	if !ok {
		return astrocore.Workspace{}, errInvalidWorkspaceKey
	}
	return selected, nil
}

func GetWorkspaces(client astrocore.CoreClient) ([]astrocore.Workspace, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrocore.Workspace{}, err
	}
	if ctx.OrganizationShortName == "" {
		return []astrocore.Workspace{}, user.ErrNoShortName
	}

	sorts := []astrocore.ListWorkspacesParamsSorts{"name:asc"}
	limit := 1000
	workspaceListParams := &astrocore.ListWorkspacesParams{
		Limit: &limit,
		Sorts: &sorts,
	}

	resp, err := client.ListWorkspacesWithResponse(httpContext.Background(), ctx.Organization, workspaceListParams)
	if err != nil {
		return []astrocore.Workspace{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astrocore.Workspace{}, err
	}

	workspaces := resp.JSON200.Workspaces

	return workspaces, nil
}
