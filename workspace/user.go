package workspace

import (
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	utab = printutil.Table{
		Padding: []int{30, 50, 50},
		Header:  []string{"NAME", "WORKSPACE ID", "EMAIL"},
	}
)

// Add a user to a workspace
func Add(workspaceUuid, email string) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserAddRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceUuid, "email": email},
	}

	r, err := req.Do()
	if err != nil {
		return err
	}
	w := r.Data.AddWorkspaceUser

	utab.AddRow([]string{w.Label, w.Uuid, email}, false)
	utab.SuccessMsg = "Successfully added user to workspace"
	utab.Print()

	return nil
}

// Remove a user from a workspace
func Remove(workspaceUuid, email string) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserRemoveRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceUuid, "email": email},
	}

	r, err := req.Do()
	if err != nil {
		return err
	}
	w := r.Data.RemoveWorkspaceUser

	utab.AddRow([]string{w.Label, w.Uuid, email}, false)
	utab.SuccessMsg = "Successfully removed user from workspace"
	utab.Print()
	return nil
}
