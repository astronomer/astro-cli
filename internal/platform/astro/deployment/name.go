package deployment

import (
	"fmt"
	"strings"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// NameError is a Deployment name that names none, or several, of the
// Deployments where it was looked up. A name is not unique, so several are
// refused with their ids rather than settled by taking one: acting on the
// first would update or delete a Deployment the run never singled out.
//
// It is a usage error (IsUsageError): the run named its Deployment in a way
// that does not pick one, and nothing ran. None of that name unwraps to
// errInvalidDeployment, the error a lookup that found nothing has always
// returned.
type NameError struct {
	// Name is the name given.
	Name string
	// Workspace is the workspace it was looked up in, "" for the whole
	// organization.
	Workspace string
	// Matches are the Deployments of that name, when there are several.
	Matches []astrov1.Deployment
	// Flag is how the run names a Deployment by id instead: "--deployment",
	// "--clone".
	Flag string
}

func (e *NameError) Error() string {
	where := "the organization"
	if e.Workspace != "" {
		where = "workspace " + e.Workspace
	}
	if len(e.Matches) == 0 {
		return fmt.Sprintf("no Deployment named %q in %s that your account or API token can see", e.Name, where)
	}
	ids := make([]string, len(e.Matches))
	for i := range e.Matches {
		ids[i] = e.Matches[i].Id
		// Looked up across the organization, the same name can be in several
		// workspaces, which is what tells the ids apart.
		if e.Workspace == "" {
			ids[i] += " in workspace " + deploymentWorkspace(&e.Matches[i])
		}
	}
	return fmt.Sprintf("%d Deployments are named %q in %s: pass %s with one's id (%s)", len(e.Matches), e.Name, where, e.Flag, strings.Join(ids, ", "))
}

// IsUsageError marks NameError as a usage error for cliout.IsUsage, which
// this package cannot import.
func (e *NameError) IsUsageError() bool { return true }

// Unwrap is errInvalidDeployment for a name that found none.
func (e *NameError) Unwrap() error {
	if len(e.Matches) == 0 {
		return errInvalidDeployment
	}
	return nil
}

// deploymentWorkspace is d's workspace, by name when the listing gave one.
func deploymentWorkspace(d *astrov1.Deployment) string {
	if d.WorkspaceName != nil && *d.WorkspaceName != "" {
		return *d.WorkspaceName
	}
	return d.WorkspaceId
}

// oneNamed is the id of the one of deployments, listed in workspace ws ("" for
// the organization), called name, for a command that acts on it --deployment.
// Of several, only the ones eligible says the command can act on count
// (hibernate and wake-up take development Deployments alone); when none of
// them is, the name is still taken, so the command says why it cannot act.
// None, or several, is a *NameError.
func oneNamed(deployments []astrov1.Deployment, name, ws string, eligible func(astrov1.Deployment) bool) (string, error) {
	named := namedIn(deployments, name)
	if eligible != nil && len(named) > 1 {
		var can []astrov1.Deployment
		for i := range named {
			if eligible(named[i]) {
				can = append(can, named[i])
			}
		}
		if len(can) > 0 {
			named = can
		}
	}
	if len(named) != 1 {
		return "", &NameError{Name: name, Workspace: ws, Matches: named, Flag: "--deployment"}
	}
	return named[0].Id, nil
}

// namedIn is every one of deployments called name, in order.
func namedIn(deployments []astrov1.Deployment, name string) []astrov1.Deployment {
	var named []astrov1.Deployment
	for i := range deployments {
		if deployments[i].Name == name {
			named = append(named, deployments[i])
		}
	}
	return named
}
