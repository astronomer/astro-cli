package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

type workspaceSelection struct {
	id   string
	quit bool
	err  error
}

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

// switchList is the switch's question before its rows are fetched, paged or
// not. The same List is checked before the fetch (List.MayAsk) and then asked,
// so the refusal a run that may not ask gets before anything is fetched is
// the one asking would give.
func switchList(paged bool) picker.List {
	about := "a workspace"
	if paged {
		about = "a workspace, or which page to show next"
	}
	return picker.List{
		Header:  []string{"NAME", "ID"},
		Ask:     []input.Option{input.About(about), input.AnsweredBy("the workspace ID as an argument")},
		Invalid: errInvalidWorkspaceKey,
	}
}

// pageKeys are the letters that move between pages on offer on page
// pageNumber, given the page size and the rows the page holds, and the hint
// that names them: no "f" or "p" on the first page, no "n" on a page that is
// not full.
func pageKeys(pageSize, pageNumber, rows int) (keys []string, hint string) {
	var names []string
	if pageNumber > 0 {
		keys = append(keys, "f", "p")
		names = append(names, "f. first", "p. previous")
	}
	if rows >= pageSize {
		keys = append(keys, "n")
		names = append(names, "n. next")
	}
	keys = append(keys, "q")
	names = append(names, "q. quit")
	return keys, "Please select one of the following options or enter index to select the row.\n" + strings.Join(names, " ")
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
	list := switchList(true)
	if err := list.MayAsk(); err != nil {
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

	// Rows are numbered on from the pages before, so the second page of ten
	// starts at 11, and answered by the numbers shown.
	list.Keys, list.Hint = pageKeys(pageSize, pageNumber, len(ws))
	list.First = pageNumber*pageSize + 1
	for i := range ws {
		list.AddRow(c.Workspace == ws[i].ID, ws[i].Label, ws[i].ID)
	}
	choice, err := list.Choose(out, os.Stdin)
	if err != nil {
		return workspaceSelection{err: err}
	}
	switch choice.Key {
	case "":
		return workspaceSelection{id: ws[choice.Row].ID}
	case "q":
		return workspaceSelection{quit: true}
	case "f":
		pageNumber = 0
	case "p":
		pageNumber--
	case "n":
		pageNumber++
	}
	return getWorkspaceSelection(pageSize, pageNumber, client, out)
}

// pickWorkspace asks for one workspace from a numbered table of all of them.
// It refuses, having fetched nothing, when this run may not ask.
func pickWorkspace(client houston.ClientInterface, out io.Writer) (string, error) {
	list := switchList(false)
	if err := list.MayAsk(); err != nil {
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
		// The picker, paged or not, is the question: its table and prompt
		// are drawn on stderr. The context it leaves is the result, on out.
		sel := getWorkspaceSelection(pageSize, 0, client, os.Stderr)

		if sel.quit {
			return nil
		}
		if sel.err != nil {
			return sel.err
		}

		id = sel.id
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
