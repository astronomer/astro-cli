package astro

// How `astro workspace create`, `update`, `delete` and `switch`, `astro
// organization switch` and `astro organization role list` publish what they
// did, in text and in json. The platform packages return it; this file is the
// only place deciding how it looks.

import (
	"bufio"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	roleClient "github.com/astronomer/astro-cli/internal/platform/astro/role"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// The -o of `workspace create|update|delete|switch` (one run is one command,
// so they share it), of `organization switch`, and of the `organization role`
// group.
var (
	workspaceLifecycleOutput string
	organizationSwitchOutput string
	organizationRoleOutput   string
)

// questionsTo is where the platform draws what it asks and the notes it
// prints along the way: the command's own writer in text, as always, and
// stderr under json, where stdout carries the one result. Nothing is asked
// under json (the run refuses instead), so on stderr there are notes only.
func questionsTo(cmd *cobra.Command, format cliout.Format, out io.Writer) io.Writer {
	if format == cliout.FormatJSON {
		return cmd.ErrOrStderr()
	}
	return out
}

// emitWorkspace publishes the Workspace a create or an update left, as
// `workspace list` shows one; in text, the line the command always printed.
func emitWorkspace(r cliout.Renderer, ws *workspace.WorkspaceInfo, line string) error {
	return r.Emit(ws, cliout.Text(func(b *bufio.Writer) { fmt.Fprintln(b, line) }))
}

// emitWorkspaceRemoval publishes a delete.
func emitWorkspaceRemoval(r cliout.Renderer, removal *workspace.Removal) error {
	return r.Emit(removal, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Astro Workspace %s was successfully deleted\n", removal.Name)
	}))
}

// emitAuditLogExport publishes the file an audit-log export wrote; in text,
// the line the command always printed.
func emitAuditLogExport(r cliout.Renderer, export *organization.AuditLogExport) error {
	return r.Emit(export, cliout.Text(func(b *bufio.Writer) { fmt.Fprintln(b, "Finished exporting logs to local GZIP file") }))
}

// emitWorkspaceSwitch publishes the Workspace a switch made current. In text
// it is the context table the switch always printed, read from the config the
// switch wrote.
func emitWorkspaceSwitch(r cliout.Renderer, ws *workspace.WorkspaceInfo) error {
	return r.Emit(ws, config.PrintCurrentCloudContext)
}

// switchedLine is what text says about the Organization a switch left.
func switchedLine(changed bool) string {
	if changed {
		return "\nSuccessfully switched organization"
	}
	return "You selected the same organization as the current one. No switch was made"
}

// emitOrganizationSwitch publishes what an Organization switch left current.
// In text, the line about the Organization, and the context table when the
// run also switched the Workspace (--workspace), as it always printed them.
func emitOrganizationSwitch(r cliout.Renderer, res *organization.SwitchResult, changed, workspaceSwitched bool) error {
	return r.Emit(res, func(w io.Writer) error {
		if err := cliout.WriteText(w, func(b *bufio.Writer) { fmt.Fprintln(b, switchedLine(changed)) }); err != nil {
			return err
		}
		if !workspaceSwitched {
			return nil
		}
		return config.PrintCurrentCloudContext(w)
	})
}

// emitRoles publishes the role list; in text, the table it always printed,
// a default role's ID cell empty.
func emitRoles(r cliout.Renderer, roles *roleClient.RoleList) error {
	return r.Emit(roles, func(w io.Writer) error {
		table := printutil.Table{
			Padding:        []int{30, 50, 10, 50, 10, 10, 10},
			DynamicPadding: true,
			Header:         []string{"NAME", "ID", "DESCRIPTION"},
		}
		for _, ro := range roles.Roles {
			table.AddRow([]string{ro.Name, ro.ID, ro.Description}, false)
		}
		return table.Print(w)
	})
}
