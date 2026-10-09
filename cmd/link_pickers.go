package cmd

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/local"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The login steps the link pickers take before they call the API. Vars so a
// test can drive a session that needs refreshing without an identity provider.
var (
	// ensureLinkLogin is the check the root's pre-run gives every cloud
	// command, `astro deploy` and `astro deployment list` among them. The core
	// tree skips that pre-run, so without this the pickers would send
	// whatever token is on disk, expired or not.
	ensureLinkLogin = astroCmd.EnsureLogin
	// refreshLinkLogin renews the token when the platform refuses it anyway.
	refreshLinkLogin = astroCmd.RefreshLogin
)

// wireLinkPickers wires the link pickers for an Astro context only. They list
// Astro Deployments and workspaces through the Astro login, so under Software
// there is nothing for them to list, and `astro link add` with no --deployment
// asks for one the way it does in a run that cannot prompt.
func wireLinkPickers(d *local.Deps, platform string, client astrov1.APIClient, out io.Writer) {
	if platform != cloudPlatform {
		return
	}
	withLinkPickers(d, client, out)
}

// withLinkPickers wires `astro link`'s Astro pickers into the core deps. They are
// the pickers `astro deploy` already asks with, through the same v1 client and
// the same current login: the Deployment list an unlinked project deploys from,
// and the workspace list `astro workspace switch` offers. cmd/local cannot
// import them itself (the core tree never reads config/), so the root wires them
// here, as it wires the rest of the platform.
func withLinkPickers(d *local.Deps, client astrov1.APIClient, out io.Writer) {
	d.CurrentWorkspace = func() string {
		ws, _ := workspace.GetCurrentWorkspace() //nolint:errcheck // no current workspace reads as none, as deploy reads it
		return ws
	}
	d.PickDeployment = func(workspaceID string, linked map[string]string) (local.PickedDeployment, error) {
		// deploy's own filter hook leaves out what the project already links.
		unlinked := func(dep astrov1.Deployment) bool {
			_, ok := linked[dep.Id]
			return !ok
		}
		var dep astrov1.Deployment
		err := withLinkLogin(client, func() error {
			// The create flow is off: linking names a Deployment that exists.
			var err error
			dep, err = deployment.GetDeployment(workspaceID, "", "", true, unlinked, client)
			return err
		})
		// With the create flow off, a workspace with Deployments that the
		// filter leaves none of is this error; an empty workspace returns no
		// Deployment and no error.
		if err != nil && strings.HasPrefix(err.Error(), deployment.NoDeploymentInWSMsg) {
			return local.PickedDeployment{}, local.ErrAllDeploymentsLinked
		}
		if err != nil {
			return local.PickedDeployment{}, err
		}
		if dep.Id == "" {
			return local.PickedDeployment{}, errors.New(deployment.NoDeploymentInWSMsg + " " + workspaceID)
		}
		return local.PickedDeployment{ID: dep.Id, Name: dep.Name, WorkspaceID: dep.WorkspaceId}, nil
	}
	d.PickWorkspace = func() (string, error) {
		var id string
		if err := input.MayAsk("Select a Workspace", input.About("a workspace"), input.AnsweredBy("--workspace")); err != nil {
			return "", err
		}
		err := withLinkLogin(client, func() error {
			// Titled the way the Deployment picker is; `astro workspace
			// switch` asks with the same table and no title.
			fmt.Fprintln(out, "Select a Workspace")
			var err error
			id, err = workspace.GetWorkspaceSelection(client, out)
			return err
		})
		return id, err
	}
}

// withLinkLogin runs call with a usable login. It runs deploy's login check
// first; a call the platform still refuses with 401 gets the token renewed and
// one more try; and a 401 after that is the session expired, named as the
// workspace link names it (docs/workspace-link.md), never as "not found".
func withLinkLogin(client astrov1.APIClient, call func() error) error {
	if err := ensureLinkLogin(client); err != nil {
		return loginCheckFailed(err)
	}
	err := call()
	if !isUnauthorized(err) {
		return err
	}
	if rerr := refreshLinkLogin(); rerr != nil {
		if isOffline(rerr) {
			return offline()
		}
		return sessionExpired()
	}
	if err = call(); isUnauthorized(err) {
		return sessionExpired()
	}
	return err
}

// isUnauthorized reports a platform 401, from the status the API error carries.
func isUnauthorized(err error) bool {
	return httputil.HasStatus(err, http.StatusUnauthorized)
}

// loginCheckFailed says why deploy's login check did not pass. Only a refused
// token is a session that expired; anything else keeps its own words. An
// ASTRO_API_TOKEN failure is the token's own message, since the login is not
// what failed, and an unreachable host is the workspace link's offline cause.
func loginCheckFailed(err error) error {
	switch {
	case astrosession.HasAPIToken():
		return err
	case isOffline(err):
		return offline()
	case isUnauthorized(err):
		return sessionExpired()
	}
	return fmt.Errorf("checking your %s login: %w", currentDomain(), err)
}

// isOffline reports an error from a request that got no response.
func isOffline(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// currentDomain is the current login's host, as a login stores it.
func currentDomain() string {
	domain, err := config.GetCurrentDomain()
	if err != nil {
		return ""
	}
	return manifest.NormalizeDomain(domain)
}

// sessionExpired is the workspace link's cause for a 401, on the current
// login's domain.
func sessionExpired() error {
	domain := currentDomain()
	if domain == "" {
		return errors.New("you are not logged in. Log in with astro login")
	}
	return fmt.Errorf("your %s session expired. Log in again with astro login %s", domain, domain)
}

// offline is the workspace link's cause for no response, on the current
// login's domain.
func offline() error {
	return fmt.Errorf("could not reach %s. Check your connection", currentDomain())
}
