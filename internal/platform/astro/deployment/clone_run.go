package deployment

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/clone"
	"github.com/astronomer/astro-cli/pkg/util"
)

// CloneSource reads the Deployment `astro deployment create --clone` copies.
// ref is a Deployment id, read from anywhere in the Organization, or else a
// name, looked up in Workspace ws. A name more than one Deployment there
// shares is refused with their ids rather than settled by taking one, and
// nothing is ever asked.
func CloneSource(ref, ws string, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	if util.IsCUID(ref) {
		return GetDeploymentByID("", ref, astroV1Client)
	}
	if ws == "" {
		return astrov1.Deployment{}, fmt.Errorf("no Workspace to look up the Deployment named %q in: pass --workspace, or its id to --clone", ref)
	}
	deployments, err := ListDeployments(ws, "", astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	var ids []string
	for i := range deployments {
		if deployments[i].Name == ref {
			ids = append(ids, deployments[i].Id)
		}
	}
	switch len(ids) {
	case 0:
		return astrov1.Deployment{}, fmt.Errorf("no Deployment named %q in Workspace %s. Pass its id to --clone to copy one from another Workspace", ref, ws)
	case 1:
		return GetDeploymentByID("", ids[0], astroV1Client)
	}
	return astrov1.Deployment{}, fmt.Errorf("%d Deployments are named %q in Workspace %s: pass one of their ids to --clone instead: %s", len(ids), ref, ws, strings.Join(ids, ", "))
}

// Clone creates a copy of src named name, in workspaceID or else src's own
// Workspace, with one create request (see clone.Request), and with wait,
// waits up to waitTime for it to become healthy, writing that wait's progress
// to progress (stderr when nil). It returns what src has that
// the copy does not.
//
// The Deployment exists whether or not it becomes healthy in time, so a wait
// that runs out returns it with the error, as Create does.
func Clone(src *astrov1.Deployment, name, workspaceID string, description *string, wait bool, waitTime time.Duration, astroV1Client astrov1.APIClient, progress io.Writer) (astrov1.Deployment, []clone.Note, error) {
	req, notes, err := clone.Request(src, name, workspaceID, description)
	if err != nil {
		return astrov1.Deployment{}, nil, err
	}
	d, err := coreCreateDeployment(src.OrganizationId, req, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, notes, err
	}
	if wait {
		if err := HealthPoll(progress, d.Id, SleepTime, TickNum, int(waitTime.Seconds()), astroV1Client); err != nil {
			return d, notes, err
		}
	}
	return d, notes, nil
}
