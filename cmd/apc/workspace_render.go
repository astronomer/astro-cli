package apc

import (
	"bufio"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// The shapes `astro workspace create|delete|list|switch|update` publish under
// --output json on APC, and the text each has always printed, drawn from the
// same values. The platform functions return what Houston said; this is the
// one place that decides how it looks.

// workspaceJSON is one APC Workspace.
//
// Houston's Workspace has a nullable description: null when it has none, ""
// when it holds an empty one, as Houston returns them. createdAt and
// updatedAt are never null in Houston's table, but come back only where the
// query asks for them: list, create, update and switch do, delete does not;
// they are null where it did not. Houston requires a label on create and update and refuses a blank one
// (
// update-workspace/), so the label is always a string.
type workspaceJSON struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Description *string `json:"description"`
	// CreatedAt and UpdatedAt are ISO 8601, as Houston gives them.
	CreatedAt *string `json:"created_at"`
	UpdatedAt *string `json:"updated_at"`
	// IsCurrent is whether it is the current context's workspace, the row
	// the list highlights; after a switch, it is.
	IsCurrent bool `json:"is_current"`
}

// workspaceListJSON is `astro workspace list`, in Houston's order.
type workspaceListJSON struct {
	Workspaces []workspaceJSON `json:"workspaces"`
}

// workspaceRemovalJSON is what `astro workspace delete` removed. Action is
// "deleted". Label is the removed record's, null if Houston answered with
// none.
type workspaceRemovalJSON struct {
	WorkspaceID string  `json:"workspace_id"`
	Label       *string `json:"label"`
	Action      string  `json:"action"`
}

// workspaceValue is s when Houston gave a value, and nil when it gave none,
// for fields Houston never holds as "" (the timestamps, the label).
func workspaceValue(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// currentWorkspaceID is the current context's workspace, or "" when there is
// no context to read one from: is_current is then false everywhere, and the
// command does not fail for it.
func currentWorkspaceID() string {
	c, err := config.GetCurrentContext()
	if err != nil {
		return ""
	}
	return c.Workspace
}

func newWorkspaceJSON(w *houston.Workspace, current string) workspaceJSON {
	return workspaceJSON{
		ID:          w.ID,
		Label:       w.Label,
		Description: w.Description,
		CreatedAt:   workspaceValue(w.CreatedAt),
		UpdatedAt:   workspaceValue(w.UpdatedAt),
		IsCurrent:   current != "" && w.ID == current,
	}
}

// workspaceTable is the NAME/ID table create, update and list print.
func workspaceTable() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

// emitWorkspace publishes the workspace create or update left, with the
// sentence each has always ended on.
func emitWorkspace(r cliout.Renderer, w *houston.Workspace, success string) error {
	return r.Emit(newWorkspaceJSON(w, currentWorkspaceID()), func(out io.Writer) error {
		tab := workspaceTable()
		tab.AddRow([]string{w.Label, w.ID}, false)
		tab.SuccessMsg = success
		return tab.Print(out)
	})
}

// emitWorkspaceList publishes the workspaces list found, highlighting the
// current one in text.
// current is the current context's workspace.
func emitWorkspaceList(r cliout.Renderer, ws []houston.Workspace, current string) error {
	list := workspaceListJSON{Workspaces: make([]workspaceJSON, 0, len(ws))}
	for i := range ws {
		list.Workspaces = append(list.Workspaces, newWorkspaceJSON(&ws[i], current))
	}
	return r.Emit(list, func(out io.Writer) error {
		tab := workspaceTable()
		for i := range ws {
			tab.AddRow([]string{ws[i].Label, ws[i].ID}, current != "" && ws[i].ID == current)
		}
		return tab.Print(out)
	})
}

// emitWorkspaceRemoval publishes what delete removed. id is the one given,
// which stands when Houston answered with no record.
func emitWorkspaceRemoval(r cliout.Renderer, id string, w *houston.Workspace) error {
	removal := workspaceRemovalJSON{WorkspaceID: id, Action: "deleted"}
	if w != nil {
		if w.ID != "" {
			removal.WorkspaceID = w.ID
		}
		removal.Label = workspaceValue(w.Label)
	}
	return r.Emit(removal, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintln(b, "\n Successfully deleted workspace")
	}))
}

// emitSwitched publishes the workspace a switch made current. Its text is
// the context the switch left, as it always was.
func emitSwitched(r cliout.Renderer, w *houston.Workspace) error {
	return r.Emit(newWorkspaceJSON(w, currentWorkspaceID()), config.PrintCurrentSoftwareContext)
}
