package instancelocate

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/pkg/util"
)

// A -d/--deployment value that names no link is read the way --deployment
// reads on the astro deployment commands: a Deployment id (util.IsCUID) is
// that Deployment, and anything else is the name of a Deployment in a
// workspace. NamedDeployment and DeploymentNameError are the half of that
// meant to be shared by every reader of such a -d: astro af reads through them
// today (DeploymentFinder), and astro api airflow is meant to, each listing
// the workspace through its own login.

// DeploymentNameError is a -d value that is no Deployment id and named no
// Deployment where it was looked for, or several, or had no workspace to be
// looked for in.
type DeploymentNameError struct {
	// Name is the value given.
	Name string
	// Workspace is where it was looked for, empty when there was nowhere.
	Workspace string
	// IDs are the Deployments of that name, when there are several.
	IDs []string
}

func (e *DeploymentNameError) Error() string {
	switch {
	case e.Workspace == "":
		return fmt.Sprintf("%q is not a Deployment id, and there is no workspace to find a Deployment of that name in: pass --workspace, or run 'astro workspace switch'", e.Name)
	case len(e.IDs) == 0:
		return fmt.Sprintf("no Deployment named %q in workspace %s", e.Name, e.Workspace)
	}
	return fmt.Sprintf("%d Deployments are named %q in workspace %s: pass -d with one's id (%s)", len(e.IDs), e.Name, e.Workspace, strings.Join(e.IDs, ", "))
}

// NotFound reports a name that found no Deployment: none of that name, or no
// workspace to look in. Inside a project that is most likely a mistyped link,
// so a caller leads with the links; several of that name is not.
func (e *DeploymentNameError) NotFound() bool { return len(e.IDs) == 0 }

// NamedDeployment is the one of deployments, the Deployments of workspace,
// called name. None, or several, is a *DeploymentNameError; an empty
// workspace is one too, since there was nowhere to look.
func NamedDeployment(deployments []astrov1.Deployment, name, workspace string) (astrov1.Deployment, error) {
	if workspace == "" {
		return astrov1.Deployment{}, &DeploymentNameError{Name: name}
	}
	var found []astrov1.Deployment
	for i := range deployments {
		if deployments[i].Name == name {
			found = append(found, deployments[i])
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	nameErr := &DeploymentNameError{Name: name, Workspace: workspace}
	for i := range found {
		nameErr.IDs = append(nameErr.IDs, found[i].Id)
	}
	return astrov1.Deployment{}, nameErr
}

// DeploymentFinder is implemented by a locator that can read a -d value no
// link declares into a Deployment id. It is optional for the reason
// GoogleChain is: a test that wires a bare lookup function has no control
// plane to list.
type DeploymentFinder interface {
	// DeploymentID is ref when it is a Deployment id, with no call made: the
	// lookup that reads its address is the one that checks it. Anything else
	// is a Deployment name, looked up in workspace, else projectWorkspace,
	// else the login's.
	DeploymentID(ctx context.Context, ref, workspace, projectWorkspace string) (string, error)
}

// The production locator finds names: cmd/local asks for the interface, and a
// locator that stopped implementing it would quietly read every name as an id.
var _ DeploymentFinder = (*locator)(nil)

// DeploymentID is the DeploymentFinder seam.
func (l *locator) DeploymentID(ctx context.Context, ref, workspace, projectWorkspace string) (string, error) {
	if util.IsCUID(ref) {
		return ref, nil
	}
	ws := workspace
	if ws == "" {
		ws = projectWorkspace
	}
	if ws == "" && l.workspace != nil {
		ws = l.workspace()
	}
	// The login comes first: a machine with none has no workspace either, and
	// logging in is the fix that also supplies one.
	token, org, err := l.identity(ctx)
	if err != nil {
		return "", err
	}
	if ws == "" {
		return "", &DeploymentNameError{Name: ref}
	}
	deployments, err := l.listDeployments(ctx, token, org, ws)
	if err != nil {
		return "", err
	}
	dep, err := NamedDeployment(deployments, ref, ws)
	if err != nil {
		return "", err
	}
	return dep.Id, nil
}

// listDeployments is every Deployment in workspace ws, under the session's
// bearer for the same reason astroDeployment puts it on its call.
func (l *locator) listDeployments(ctx context.Context, token, org, ws string) ([]astrov1.Deployment, error) {
	limit := 1000
	return pagination.Collect("deployments", func(offset int) ([]astrov1.Deployment, int, error) {
		params := &astrov1.ListDeploymentsParams{Limit: &limit, Offset: &offset, WorkspaceIds: &[]string{ws}}
		resp, err := l.deployments.ListDeploymentsWithResponse(ctx, org, params, bearer(token))
		if err != nil {
			return nil, 0, fmt.Errorf("could not reach Astro to list the Deployments in workspace %s — check your connection: %w", ws, err)
		}
		if resp.JSON200 == nil {
			return nil, 0, listOutage(l.domain, ws, resp)
		}
		return resp.JSON200.Deployments, resp.JSON200.TotalCount, nil
	})
}

// listOutage names what the control plane said to a listing, in the words
// deploymentOutage uses for a lookup.
func listOutage(domain, ws string, resp *astrov1.ListDeploymentsResponse) error {
	status := 0
	if resp.HTTPResponse != nil {
		status = resp.HTTPResponse.StatusCode
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w (listing the Deployments in workspace %s)", astrosession.Rejected(domain), ws)
	case http.StatusForbidden:
		return fmt.Errorf("you do not have access to workspace %s — ask a workspace admin to grant it", ws)
	}
	detail := ""
	if resp.HTTPResponse != nil {
		if apiErr := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); apiErr != nil {
			detail = ": " + apiErr.Error()
		}
	}
	return fmt.Errorf("Astro could not list the Deployments in workspace %s%s", ws, detail)
}

// Workspace reads the current workspace out of the login for domain, or out
// of the current login context when domain is empty: where a Deployment name
// is looked up when nothing else says. Empty when there is none.
func Workspace(domain string) string {
	if domain == "" {
		ctx, err := config.GetCurrentContext()
		if err != nil {
			return ""
		}
		return ctx.Workspace
	}
	c := config.Context{Domain: domain}
	ctx, err := c.GetContext()
	if err != nil {
		return ""
	}
	return ctx.Workspace
}
