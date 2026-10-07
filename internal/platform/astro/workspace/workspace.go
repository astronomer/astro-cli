package workspace

import (
	httpContext "context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	errInvalidWorkspaceKey = errors.New("invalid workspace selection")
	ErrInvalidName         = errors.New("no name provided for the workspace. Retry with a valid name")
	ErrInvalidTokenName    = errors.New("no name provided for the workspace token. Retry with a valid name")
	ErrWorkspaceNotFound   = errors.New("no workspace was found for the ID you provided")
	ErrNoWorkspaceExists   = errors.New("no workspace was found in your organization")
	ErrWrongEnforceInput   = errors.New("the input to the `--enforce-cicd` flag")
)

var workspaceTableConfig = output.BuildTableConfig(
	[]output.Column[WorkspaceInfo]{
		{Header: "NAME", Value: func(w WorkspaceInfo) string { return w.Name }},
		{Header: "ID", Value: func(w WorkspaceInfo) string { return w.ID }},
	},
	func(d any) []WorkspaceInfo { return d.(*WorkspaceList).Workspaces },
	output.WithColorRow(func(w WorkspaceInfo) bool { return w.IsCurrent }, [2]string{"\033[1;32m", "\033[0m"}),
	output.WithPadding([]int{44, 50}),
)

// GetCurrentWorkspace gets the current workspace set in context config
// Returns a string representing the current workspace and an error if it doesn't exist
func GetCurrentWorkspace() (string, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}

	if c.Workspace == "" {
		return "", errors.New("current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")
	}

	return c.Workspace, nil
}

// ListData returns workspace list data for structured output
func ListData(client astrov1.APIClient) (*WorkspaceList, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	ws, err := GetWorkspaces(client)
	if err != nil {
		return nil, err
	}

	result := &WorkspaceList{
		Workspaces: make([]WorkspaceInfo, 0, len(ws)),
	}

	for i := range ws {
		isCurrent := c.Workspace == ws[i].Id
		result.Workspaces = append(result.Workspaces, WorkspaceInfo{
			Name:      ws[i].Name,
			ID:        ws[i].Id,
			IsCurrent: isCurrent,
		})
	}

	return result, nil
}

// ListWithFormat lists workspaces with the specified output format
func ListWithFormat(client astrov1.APIClient, r output.Emitter) error {
	return output.PrintData(
		func() (*WorkspaceList, error) { return ListData(client) },
		workspaceTableConfig, r,
	)
}

var GetWorkspaceSelection = func(client astrov1.APIClient, out io.Writer) (string, error) {
	w, err := pickWorkspace(client, out)
	if err != nil {
		return "", err
	}
	return w.Id, nil
}

// pickWorkspace asks which of the Organization's Workspaces is meant, drawing
// the menu on out. opts say more about the question, for a refusal to name.
func pickWorkspace(client astrov1.APIClient, out io.Writer, opts ...input.Option) (*astrov1.Workspace, error) {
	// Refused before anything is listed or drawn: a run that cannot ask
	// prints nothing it would then have to explain.
	list := picker.List{
		Header:  []string{"NAME", "ID"},
		Ask:     append([]input.Option{input.About("a workspace")}, opts...),
		Invalid: errInvalidWorkspaceKey,
	}
	if err := input.MayAsk("\n> ", list.Ask...); err != nil {
		return nil, err
	}

	var c config.Context
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	ws, err := GetWorkspaces(client)
	if err != nil {
		return nil, err
	}

	for i := range ws {
		list.AddRow(c.Workspace == ws[i].Id, ws[i].Name, ws[i].Id)
	}
	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return nil, err
	}
	return &ws[i], nil
}

// NameOrIDAnswer is what answers the question a switch naming no Workspace
// asks.
const NameOrIDAnswer = "the workspace name or ID as an argument"

// Switch makes the Workspace named current, and prints the context table on
// out, which is what logging in shows after it picks a Workspace.
func Switch(workspaceNameOrID string, client astrov1.APIClient, out io.Writer) error {
	if _, err := SwitchTo(workspaceNameOrID, client, out); err != nil {
		return err
	}
	return config.PrintCurrentCloudContext(out)
}

// SwitchTo makes the Workspace named, by name or id, current, and returns it.
// With no name it asks which, drawing the menu on out.
func SwitchTo(workspaceNameOrID string, client astrov1.APIClient, out io.Writer) (*WorkspaceInfo, error) {
	var picked *astrov1.Workspace
	if workspaceNameOrID == "" {
		w, err := pickWorkspace(client, out, input.AnsweredBy(NameOrIDAnswer))
		if err != nil {
			return nil, err
		}
		picked = w
	} else {
		ws, err := GetWorkspaces(client)
		if err != nil {
			return nil, err
		}
		for i := range ws {
			if ws[i].Name == workspaceNameOrID || ws[i].Id == workspaceNameOrID {
				picked = &ws[i]
			}
		}

		if picked == nil {
			return nil, errors.New("workspace id/name could not be found")
		}
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	err = c.SetContextKey("workspace", picked.Id)
	if err != nil {
		return nil, err
	}

	err = c.SetContextKey("last_used_workspace", picked.Id)
	if err != nil {
		return nil, err
	}

	err = c.SetOrganizationContext(c.Organization, c.OrganizationProduct)
	if err != nil {
		return nil, err
	}

	return &WorkspaceInfo{Name: picked.Name, ID: picked.Id, IsCurrent: true}, nil
}

// Current returns the current Workspace, as `workspace list` shows it, or nil
// when the context names none of the current Organization's Workspaces: none
// at all, or one an Organization switch left behind.
func Current(client astrov1.APIClient) (*WorkspaceInfo, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if c.Workspace == "" {
		return nil, nil
	}
	ws, err := GetWorkspaces(client)
	if err != nil {
		return nil, err
	}
	for i := range ws {
		if ws[i].Id == c.Workspace {
			return &WorkspaceInfo{Name: ws[i].Name, ID: ws[i].Id, IsCurrent: true}, nil
		}
	}
	return nil, nil
}

// info is w as `workspace list` shows it.
func info(w *astrov1.Workspace) (WorkspaceInfo, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return WorkspaceInfo{}, err
	}
	return WorkspaceInfo{Name: w.Name, ID: w.Id, IsCurrent: c.Workspace == w.Id}, nil
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

// Create creates a Workspace in the current Organization, and returns it as
// `workspace list` shows it.
func Create(name, description, enforceCD string, client astrov1.APIClient) (*WorkspaceInfo, error) {
	if name == "" {
		return nil, ErrInvalidName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	enforce, err := validateEnforceCD(enforceCD)
	if err != nil {
		return nil, err
	}
	workspaceCreateRequest := astrov1.CreateWorkspaceJSONRequestBody{
		CicdEnforcedDefault: &enforce,
		Description:         &description,
		Name:                name,
	}
	resp, err := client.CreateWorkspaceWithResponse(httpContext.Background(), ctx.Organization, workspaceCreateRequest)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("something went wrong: the API did not return the Workspace %s it created", name)
	}
	created, err := info(resp.JSON200)
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// Update updates the Workspace id names, or, with no id, the one picked from
// a menu drawn on out, and returns what it did.
func Update(id, name, description, enforceCD string, out io.Writer, client astrov1.APIClient) (*Updated, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	workspace, err := findWorkspace(id, "the workspace you would like to update", out, client)
	if err != nil {
		return nil, err
	}
	workspaceID := workspace.Id

	workspaceUpdateRequest := astrov1.UpdateWorkspaceRequest{}

	if name == "" {
		workspaceUpdateRequest.Name = workspace.Name
	} else {
		workspaceUpdateRequest.Name = name
	}

	if description == "" {
		if workspace.Description != nil {
			workspaceUpdateRequest.Description = *workspace.Description
		}
	} else {
		workspaceUpdateRequest.Description = description
	}
	if enforceCD == "" {
		workspaceUpdateRequest.CicdEnforcedDefault = workspace.CicdEnforcedDefault
	} else {
		enforce, err := validateEnforceCD(enforceCD)
		if err != nil {
			return nil, err
		}
		workspaceUpdateRequest.CicdEnforcedDefault = enforce
	}
	resp, err := client.UpdateWorkspaceWithResponse(httpContext.Background(), ctx.Organization, workspaceID, workspaceUpdateRequest)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("something went wrong: the API did not return the Workspace %s it updated", workspace.Name)
	}
	updated, err := info(resp.JSON200)
	if err != nil {
		return nil, err
	}
	return &Updated{Workspace: updated, PreviousName: workspace.Name}, nil
}

// Delete deletes the Workspace id names, or, with no id, the one picked from
// a menu drawn on out, after asking unless yes. It returns what it deleted,
// or nil when the question was declined, which it says on out.
func Delete(id string, yes bool, out io.Writer, client astrov1.APIClient) (*Removal, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	// A run that cannot ask is refused before anything is listed or picked:
	// the question it would end on is one only --yes answers.
	if !yes {
		if err := input.MayAsk("", input.About("confirmation to delete the Workspace"), input.AnsweredBy("--yes")); err != nil {
			return nil, err
		}
	}
	workspace, err := findWorkspace(id, "the workspace you would like to delete", out, client)
	if err != nil {
		return nil, err
	}
	if !yes {
		ok, err := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to delete the %s Workspace (%s)? This cannot be undone.", ansi.Bold(workspace.Name), workspace.Id),
			input.AnsweredBy("--yes"))
		if err != nil {
			return nil, err
		}
		if !ok {
			fmt.Fprintln(out, "Canceling Workspace deletion")
			return nil, nil
		}
	}
	resp, err := client.DeleteWorkspaceWithResponse(httpContext.Background(), ctx.Organization, workspace.Id)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return &Removal{WorkspaceID: workspace.Id, Name: workspace.Name, Action: ActionDeleted}, nil
}

// IDAnswer is what answers the question an update or a delete naming no
// Workspace asks.
const IDAnswer = "the workspace ID as an argument"

// findWorkspace returns the Workspace id names, or, with no id, the one picked
// from a menu drawn on out; about says what the menu asks for.
func findWorkspace(id, about string, out io.Writer, client astrov1.APIClient) (astrov1.Workspace, error) {
	workspaces, err := GetWorkspaces(client)
	if err != nil {
		return astrov1.Workspace{}, err
	}
	if id == "" {
		// The question's refusal first: with more than one Workspace and no
		// way to ask, that is the answer, not that there are none.
		workspace, err := selectWorkspace(workspaces, about, out)
		if err != nil {
			return astrov1.Workspace{}, err
		}
		if workspace.Id == "" {
			return astrov1.Workspace{}, ErrNoWorkspaceExists
		}
		return workspace, nil
	}
	for i := range workspaces {
		if workspaces[i].Id == id {
			return workspaces[i], nil
		}
	}
	return astrov1.Workspace{}, ErrWorkspaceNotFound
}

func selectWorkspace(workspaces []astrov1.Workspace, about string, out io.Writer) (astrov1.Workspace, error) {
	if len(workspaces) == 0 {
		return astrov1.Workspace{}, nil
	}

	if len(workspaces) == 1 {
		fmt.Fprintln(out, "Only one Workspace was found. Using the following Workspace by default: \n"+
			fmt.Sprintf("\n Workspace Name: %s", ansi.Bold(workspaces[0].Name))+
			fmt.Sprintf("\n Workspace ID: %s\n", ansi.Bold(workspaces[0].Id)))

		return workspaces[0], nil
	}

	list := picker.List{
		Title:   "\nPlease select " + about + ":",
		Header:  []string{"WORKSPACENAME", "ID", "CICD ENFORCEMENT"},
		Ask:     []input.Option{input.About(about), input.AnsweredBy(IDAnswer)},
		Invalid: errInvalidWorkspaceKey,
	}
	for i := range workspaces {
		list.AddRow(false, workspaces[i].Name, workspaces[i].Id, strconv.FormatBool(workspaces[i].CicdEnforcedDefault))
	}
	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return astrov1.Workspace{}, err
	}
	return workspaces[i], nil
}

// GetWorkspaces returns every Workspace in the current Organization, paging
// through the API as needed.
func GetWorkspaces(client astrov1.APIClient) ([]astrov1.Workspace, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrov1.Workspace{}, err
	}

	sorts := []astrov1.ListWorkspacesParamsSorts{"name:asc"}
	return pagination.Collect("workspaces", func(offset int) ([]astrov1.Workspace, int, error) {
		pageSize := 1000
		params := &astrov1.ListWorkspacesParams{Limit: &pageSize, Offset: &offset, Sorts: &sorts}
		resp, err := client.ListWorkspacesWithResponse(httpContext.Background(), ctx.Organization, params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		return resp.JSON200.Workspaces, resp.JSON200.TotalCount, nil
	})
}
