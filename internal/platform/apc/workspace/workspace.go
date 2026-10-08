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
)

type workspaceSelection struct {
	id   string
	quit bool
	err  error
}

var errInvalidWorkspaceKey = errors.New("invalid workspace selection")

var errWorkspaceContextNotSet = errors.New("current workspace context not set, you can switch to a workspace with \n\tastro workspace switch WORKSPACEID")

// errNoWorkspace is a mutation answered with no workspace and no error. Houston
// answers createWorkspace and updateWorkspace with the record Prisma wrote,
// and an update of a workspace that does not exist throws rather than
// answering null. So this is what is left if that ever changes: an error,
// where it used to panic printing a nil workspace.
var errNoWorkspace = errors.New("the platform answered with no workspace")

// Create creates a workspace, and returns it as Houston stored it.
func Create(label, desc string, client houston.ClientInterface) (*houston.Workspace, error) {
	w, err := houston.Call(client.CreateWorkspace)(houston.CreateWorkspaceRequest{Label: label, Description: desc})
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errNoWorkspace
	}
	return w, nil
}

// List returns every workspace the login can see, in Houston's order.
func List(client houston.ClientInterface) ([]houston.Workspace, error) {
	return houston.Call(client.ListWorkspaces)(nil)
}

// Delete deletes a workspace, and returns the record Houston removed: its
// id, label and description. Houston fails, rather than answering null, on
// a workspace that does not exist or still has Deployments, so nil comes
// back only if that ever changes.
func Delete(id string, client houston.ClientInterface) (*houston.Workspace, error) {
	return houston.Call(client.DeleteWorkspace)(id)
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
// letters to move between pages. pageNumber counts from 0, and goes to the
// Houston at houstonVersion in its own numbering. A run that may not ask
// refuses before it fetches or prints anything.
func getWorkspaceSelection(pageSize, pageNumber int, houstonVersion string, client houston.ClientInterface, out io.Writer) workspaceSelection {
	if pageSize <= 0 {
		id, err := pickWorkspace(client, out)
		return workspaceSelection{id: id, err: err}
	}
	list := switchList(true)
	if err := list.MayAsk(); err != nil {
		return workspaceSelection{err: err}
	}

	ws, err := houston.Call(client.PaginatedListWorkspaces)(houston.PaginatedListWorkspaceRequest{
		PageSize:   pageSize,
		PageNumber: houston.WorkspacesPageNumber(pageNumber, houstonVersion),
	})
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
	return getWorkspaceSelection(pageSize, pageNumber, houstonVersion, client, out)
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

// MayPick is the refusal the switch's picker would give, paged or not, in a
// run that may not ask, and nil in one that may. A caller with notes of its
// own to print first checks it, so a refused run prints nothing.
func MayPick(paged bool) error {
	list := switchList(paged)
	return list.MayAsk()
}

// Switch makes a workspace the current context's, and returns it as Houston
// has it. With no id, it asks which: from a numbered table of every one when
// pageSize is 0, and otherwise a page at a time, numbering pages for the
// Houston at houstonVersion. A person who quits the picker switches nothing:
// Switch returns quit and no workspace.
func Switch(id string, pageSize int, houstonVersion string, client houston.ClientInterface) (w *houston.Workspace, quit bool, err error) {
	if id == "" {
		// The picker, paged or not, is the question: its table and prompt
		// are drawn on stderr.
		sel := getWorkspaceSelection(pageSize, 0, houstonVersion, client, os.Stderr)

		if sel.quit {
			return nil, true, nil
		}
		if sel.err != nil {
			return nil, false, sel.err
		}

		id = sel.id
	}
	// validate workspace
	w, err = houston.Call(client.ValidateWorkspaceID)(id)
	if err != nil {
		return nil, false, fmt.Errorf("workspace id is not valid: %w", err)
	}
	// The client turns Houston's null into ErrWorkspaceNotFound; a nil with
	// no error is no workspace to switch to, so the context stays as it was.
	if w == nil {
		return nil, false, errNoWorkspace
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, false, err
	}

	c.Workspace = id
	if err := c.SetContext(); err != nil {
		return nil, false, err
	}
	return w, false, nil
}

// Update updates a workspace, and returns it as it now is.
func Update(id string, client houston.ClientInterface, args map[string]string) (*houston.Workspace, error) {
	w, err := houston.Call(client.UpdateWorkspace)(houston.UpdateWorkspaceRequest{WorkspaceID: id, Args: args})
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errNoWorkspace
	}
	return w, nil
}
