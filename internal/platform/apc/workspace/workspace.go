package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

type workspacePaginationOptions struct {
	pageSize      int
	pageNumber    int
	quit          bool
	userSelection int
}

type workspaceSelection struct {
	id   string
	quit bool
	err  error
}

const (
	defaultWorkspacePaginationOptions      = "f. first p. previous n. next q. quit\n> "
	workspacePaginationWithoutNextOptions  = "f. first p. previous q. quit\n> "
	workspacePaginationWithNextQuitOptions = "n. next q. quit\n> "
	workspacePaginationWithQuitOptions     = "q. quit\n> "
)

var errInvalidWorkspaceKey = errors.New("invalid workspace selection")

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
func Create(label, desc string, client houston.ClientInterface, out io.Writer) error {
	w, err := houston.Call(client.CreateWorkspace)(houston.CreateWorkspaceRequest{Label: label, Description: desc})
	if err != nil {
		return err
	}

	tab := newTableOut()
	tab.AddRow([]string{w.Label, w.ID}, false)
	tab.SuccessMsg = "\n Successfully created workspace"
	tab.Print(out) //nolint:errcheck // best-effort render to the terminal

	return nil
}

// List all workspaces
func List(client houston.ClientInterface, out io.Writer) error {
	ws, err := houston.Call(client.ListWorkspaces)(nil)
	if err != nil {
		return err
	}

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

	tab.Print(out) //nolint:errcheck // best-effort render to the terminal

	return nil
}

// Delete a workspace by id
func Delete(id string, client houston.ClientInterface, out io.Writer) error {
	_, err := houston.Call(client.DeleteWorkspace)(id)
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

var GetWorkspaceSelectionID = func(client houston.ClientInterface, out io.Writer) (string, error) {
	var c config.Context
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}

	ws, err := houston.Call(client.ListWorkspaces)(c.Organization)
	if err != nil {
		return "", err
	}

	list := picker.List{
		Header:  []string{"NAME", "ID"},
		Ask:     []input.Option{input.About("a workspace")},
		Invalid: errInvalidWorkspaceKey,
	}
	for i := range ws {
		list.AddRow(c.Workspace == ws[i].ID, ws[i].Label, ws[i].ID)
	}
	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return "", err
	}
	return ws[i].ID, nil
}

// switchAsk and pagedSwitchAsk describe the switch's question to a run that
// may not ask it.
var (
	switchAsk      = []input.Option{input.About("a workspace"), input.AnsweredBy("the workspace ID as an argument")}
	pagedSwitchAsk = []input.Option{input.About("a workspace, or which page to show next"), input.AnsweredBy("the workspace ID as an argument")}
)

// workspacesPromptPaginatedOption asks for a row of the page shown, or the page
// to show next, given the page size, the page number and the rows on this
// page. Rows are numbered on from the pages before, so the second page of ten
// starts at 11. Anything but a letter on offer or one of this page's row
// numbers, written exactly, fails with errInvalidWorkspaceKey.
var workspacesPromptPaginatedOption = func(pageSize, pageNumber, totalRecord int) (workspacePaginationOptions, error) {
	gotoOptionMessage := defaultWorkspacePaginationOptions
	gotoOptions := make(map[string]workspacePaginationOptions)
	gotoOptions["f"] = workspacePaginationOptions{pageSize: pageSize, quit: false, pageNumber: 0, userSelection: 0}
	gotoOptions["p"] = workspacePaginationOptions{pageSize: pageSize, quit: false, pageNumber: pageNumber - 1, userSelection: 0}
	gotoOptions["n"] = workspacePaginationOptions{pageSize: pageSize, quit: false, pageNumber: pageNumber + 1, userSelection: 0}
	gotoOptions["q"] = workspacePaginationOptions{pageSize: pageSize, quit: true, pageNumber: pageNumber, userSelection: 0}

	if totalRecord < pageSize {
		delete(gotoOptions, "n")
		gotoOptionMessage = workspacePaginationWithoutNextOptions
	}

	if pageNumber == 0 {
		delete(gotoOptions, "p")
		delete(gotoOptions, "f")
		gotoOptionMessage = workspacePaginationWithNextQuitOptions
	}

	if pageNumber == 0 && totalRecord < pageSize {
		gotoOptionMessage = workspacePaginationWithQuitOptions
	}

	in, err := input.Text("\n\nPlease select one of the following options or enter index to select the row.\n"+gotoOptionMessage, pagedSwitchAsk...)
	if err != nil {
		return workspacePaginationOptions{}, err
	}
	if value, found := gotoOptions[in]; found {
		return value, nil
	}
	offset := pageSize * pageNumber
	if n, ok := picker.Number(in, offset+1, offset+totalRecord); ok {
		userSelection := gotoOptions["q"]
		userSelection.userSelection = n - offset
		return userSelection, nil
	}
	return workspacePaginationOptions{}, errInvalidWorkspaceKey
}

// getWorkspaceSelection asks which workspace to switch to: from a numbered
// table of every one when pageSize is 0, and otherwise a page at a time, with
// letters to move between pages. A run that may not ask refuses before it
// fetches or prints anything.
func getWorkspaceSelection(pageSize, pageNumber int, client houston.ClientInterface, out io.Writer) workspaceSelection {
	if pageSize <= 0 {
		id, err := pickWorkspace(client, out)
		return workspaceSelection{id: id, err: err}
	}
	if err := input.MayAsk("\n> ", pagedSwitchAsk...); err != nil {
		return workspaceSelection{err: err}
	}

	ws, err := houston.Call(client.PaginatedListWorkspaces)(houston.PaginatedListWorkspaceRequest{PageSize: pageSize, PageNumber: pageNumber})
	if err != nil {
		return workspaceSelection{err: err}
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return workspaceSelection{err: err}
	}

	tab := newTableOut()
	tab.GetUserInput = true
	for i := range ws {
		tab.AddRow([]string{ws[i].Label, ws[i].ID}, c.Workspace == ws[i].ID)
	}
	if err := tab.PrintWithPageNumber(pageNumber*pageSize, out); err != nil {
		return workspaceSelection{err: fmt.Errorf("unable to print with page number: %w", err)}
	}

	selectedOption, err := workspacesPromptPaginatedOption(pageSize, pageNumber, len(ws))
	if err != nil {
		return workspaceSelection{err: err}
	}
	if selectedOption.quit {
		if selectedOption.userSelection == 0 {
			return workspaceSelection{quit: true}
		}
		return workspaceSelection{id: ws[selectedOption.userSelection-1].ID}
	}
	return getWorkspaceSelection(selectedOption.pageSize, selectedOption.pageNumber, client, out)
}

// pickWorkspace asks for one workspace from a numbered table of all of them.
// It refuses, having fetched nothing, when this run may not ask.
func pickWorkspace(client houston.ClientInterface, out io.Writer) (string, error) {
	if err := input.MayAsk("\n> ", switchAsk...); err != nil {
		return "", err
	}
	ws, err := houston.Call(client.ListWorkspaces)(nil)
	if err != nil {
		return "", err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}
	list := picker.List{
		Header:  []string{"NAME", "ID"},
		Ask:     switchAsk,
		Invalid: errInvalidWorkspaceKey,
	}
	for i := range ws {
		list.AddRow(c.Workspace == ws[i].ID, ws[i].Label, ws[i].ID)
	}
	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return "", err
	}
	return ws[i].ID, nil
}

// Switch switches workspaces
func Switch(id string, pageSize int, client houston.ClientInterface, out io.Writer) error {
	if id == "" {
		workspaceSelection := getWorkspaceSelection(pageSize, 0, client, out)

		if workspaceSelection.quit {
			return nil
		}
		if workspaceSelection.err != nil {
			return workspaceSelection.err
		}

		id = workspaceSelection.id
	}
	// validate workspace
	_, err := houston.Call(client.ValidateWorkspaceID)(id)
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

	err = config.PrintCurrentSoftwareContext(out)
	return err
}

// Update an APC workspace
func Update(id string, client houston.ClientInterface, out io.Writer, args map[string]string) error {
	// validate workspace
	w, err := houston.Call(client.UpdateWorkspace)(houston.UpdateWorkspaceRequest{WorkspaceID: id, Args: args})
	if err != nil {
		return err
	}

	tab := newTableOut()
	tab.AddRow([]string{w.Label, w.ID}, false)
	tab.SuccessMsg = "\n Successfully updated workspace"
	tab.Print(out) //nolint:errcheck // best-effort render to the terminal

	return nil
}
