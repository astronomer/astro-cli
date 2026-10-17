package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/astronomer/astro-cli/cmd/local"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/deployment"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/workspace"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/picker"
)

// The streams the APC Deployment picker asks on: stderr, so stdout carries
// only the command's result, as APC deploy's picker does. Vars so a test can
// script the answer.
var (
	apcLinkPickIn  io.Reader = os.Stdin
	apcLinkPickOut io.Writer = os.Stderr
)

// errInvalidAPCLinkSelection is the APC Deployment picker's answer to a
// choice that is not a row number.
var errInvalidAPCLinkSelection = errors.New("invalid Deployment selected")

// withAPCLinking wires `astro link add` for an Astro Private Cloud context: the
// context's domain, which every apc link it writes is on; its current
// workspace; and a picker over a workspace's Deployments, listed through the
// Houston client the APC commands use. A context whose domain cannot be read
// wires nothing, and add then links as it would under Astro.
//
// It runs as the root is built, for every command, so it reads only the
// current domain, which is the config's context key, never the login:
// GetCurrentContext resolves the login, which can reach the OS keyring or the
// secrets vault. The workspace and the Deployments are read inside the
// closures, when astro link add asks (TestAPCLinkingReadsNoLogin).
func withAPCLinking(d *local.Deps, client houston.ClientInterface) {
	domain, err := config.GetCurrentDomain()
	if err != nil || domain == "" {
		return
	}
	d.APC = &local.APCLinking{
		Domain: manifest.NormalizeAPCDomain(domain),
		CurrentWorkspace: func() (string, error) {
			ws, err := workspace.GetCurrentWorkspace()
			if errors.Is(err, workspace.ErrWorkspaceContextNotSet) {
				return "", nil
			}
			return ws, err
		},
		PickDeployment: func(workspaceID string, linked map[string]string) (local.PickedDeployment, error) {
			return pickAPCDeployment(client, workspaceID, linked)
		},
	}
}

// pickAPCDeployment lists the Deployments of workspace ws, leaves out the ones
// linked names, and asks which one to link.
func pickAPCDeployment(client houston.ClientInterface, ws string, linked map[string]string) (local.PickedDeployment, error) {
	list := picker.List{
		Title:   "Select a Deployment to link",
		Header:  []string{"DEPLOYMENT NAME", "RELEASE NAME", "DEPLOYMENT ID"},
		Ask:     []input.Option{input.About("a Deployment"), input.AnsweredBy("--deployment")},
		Invalid: errInvalidAPCLinkSelection,
	}
	// Refused before Houston is asked, as the Astro pickers are.
	if err := list.MayAsk(); err != nil {
		return local.PickedDeployment{}, err
	}
	all, err := deployment.GetDeployments(ws, client)
	if err != nil {
		return local.PickedDeployment{}, err
	}
	if len(all) == 0 {
		return local.PickedDeployment{}, fmt.Errorf("no Deployments in workspace %s to link", ws)
	}
	var offered []houston.Deployment
	for i := range all {
		if _, ok := linked[all[i].ID]; !ok {
			offered = append(offered, all[i])
		}
	}
	if len(offered) == 0 {
		return local.PickedDeployment{}, local.ErrAllDeploymentsLinked
	}
	for i := range offered {
		list.AddRow(false, offered[i].Label, offered[i].ReleaseName, offered[i].ID)
	}
	i, err := list.Pick(apcLinkPickOut, apcLinkPickIn)
	if err != nil {
		return local.PickedDeployment{}, err
	}
	return local.PickedDeployment{ID: offered[i].ID, Name: offered[i].Label, WorkspaceID: offered[i].Workspace.ID}, nil
}
