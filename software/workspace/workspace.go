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
	tab.Print(out)

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

	tab.Print(out)

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

// workspacesPromptPaginatedOption Show pagination option based on page size and total record
var workspacesPromptPaginatedOption = func(pageSize, pageNumber, totalRecord int) workspacePaginationOptions {
	for {
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

		in := input.Text("\n\nPlease select one of the following options or enter index to select the row.\n" + gotoOptionMessage)
		value, found := gotoOptions[in]
		i, err := strconv.ParseInt(in, 10, 8) //nolint:gomnd

		if found {
			return value
		} else if err == nil && int(i) > pageSize*pageNumber {
			userSelection := gotoOptions["q"]
			userSelection.userSelection = int(i) - pageSize*pageNumber
			return userSelection
		}
		fmt.Print("\nInvalid option")
	}
}

func getWorkspaceSelection(pageSize, pageNumber int, client houston.ClientInterface, out io.Writer) workspaceSelection {
	tab := newTableOut()
	tab.GetUserInput = true
	var ws []houston.Workspace
	var err error

	if pageSize > 0 {
		ws, err = houston.Call(client.PaginatedListWorkspaces)(houston.PaginatedListWorkspaceRequest{PageSize: pageSize, PageNumber: pageNumber})
	} else {
		ws, err = houston.Call(client.ListWorkspaces)(nil)
	}
	if err != nil {
		return workspaceSelection{id: "", quit: false, err: err}
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return workspaceSelection{id: "", quit: false, err: err}
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

	tabPrintErr := tab.PrintWithPageNumber(pageNumber*pageSize, out)
	if tabPrintErr != nil {
		return workspaceSelection{id: "", quit: false, err: fmt.Errorf("unable to print with page number: %w", tabPrintErr)}
	}
	totalRecords := len(ws)

	if pageSize > 0 {
		selectedOption := workspacesPromptPaginatedOption(pageSize, pageNumber, totalRecords)
		if selectedOption.quit {
			if selectedOption.userSelection == 0 {
				return workspaceSelection{id: "", quit: true, err: nil}
			}
			return workspaceSelection{id: ws[selectedOption.userSelection-1].ID, quit: false, err: nil}
		}
		return getWorkspaceSelection(selectedOption.pageSize, selectedOption.pageNumber, client, out)
	}

	in := input.Text("\n> ")
	i, err := strconv.ParseInt(in, 10, 64) //nolint:gomnd
	if err != nil {
		return workspaceSelection{id: "", quit: false, err: fmt.Errorf("cannot parse %s to int: %w", in, err)}
	}
	return workspaceSelection{id: ws[i-1].ID, quit: false, err: nil}
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

// Update an astronomer workspace
func Update(id string, client houston.ClientInterface, out io.Writer, args map[string]string) error {
	// validate workspace
	w, err := houston.Call(client.UpdateWorkspace)(houston.UpdateWorkspaceRequest{WorkspaceID: id, Args: args})
	if err != nil {
		return err
	}

	tab := newTableOut()
	tab.AddRow([]string{w.Label, w.ID}, false)
	tab.SuccessMsg = "\n Successfully updated workspace"
	tab.Print(out)

	return nil
}
