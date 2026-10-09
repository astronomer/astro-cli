package instancelocate

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// astroBaseURL reads the Deployment's web server URL from the control plane,
// only when a command is about to talk to that Airflow.
func (l *locator) astroBaseURL(ctx context.Context, i instances.Instance) (string, error) {
	deployment, err := l.astroDeployment(ctx, i)
	if err != nil {
		return "", err
	}
	url := deployment.WebServerAirflowApiUrl
	if url == "" {
		return "", fmt.Errorf("Astro Deployment %s (%q) has no Airflow API URL yet — it may still be starting", i.Link.Deployment, i.Name)
	}
	return url, nil
}

// astroDeployment reads the Deployment an astro link names.
//
// The identity is resolved before the call rather than after it because a
// machine with none gets a better sentence from us than from an API that was
// never reached. It is then put on the request, so ASTRO_API_TOKEN outranks
// whatever a stale login context holds — the same precedence the credential
// path uses, since the two must not disagree about who is calling.
func (l *locator) astroDeployment(ctx context.Context, i instances.Instance) (*astrov1.Deployment, error) {
	deploymentID := i.Link.Deployment
	if deploymentID == "" {
		return nil, fmt.Errorf("deployment %q names no Deployment: set deployment = '<id>' on the link", i.Name)
	}
	if l.session == nil {
		return nil, astrosession.ErrLoggedOut
	}
	token, err := l.session(ctx)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, astrosession.ErrLoggedOut
	}
	org, err := l.organization()
	if err != nil {
		return nil, err
	}
	resp, err := l.deployments.GetDeploymentWithResponse(ctx, org, deploymentID, bearer(token))
	if err != nil {
		// The request never reached an answer, which on this path means the
		// machine could not get to Astro at all.
		return nil, fmt.Errorf("could not reach Astro to look up %q — check your connection: %w", i.Name, err)
	}
	if resp.JSON200 == nil {
		return nil, deploymentOutage(l.domain, i, deploymentID, resp)
	}
	return resp.JSON200, nil
}

// WhyUnavailable asks Astro why an astro link's Airflow is not answering. It
// is the Diagnoser seam. nil means Astro has nothing to add: the lookup failed
// too, or the status is one Astro does not know.
func (l *locator) WhyUnavailable(ctx context.Context, i instances.Instance) error {
	if i.Kind != instances.KindAstro {
		return nil
	}
	deployment, err := l.astroDeployment(ctx, i)
	if err != nil {
		return nil //nolint:nilerr // the Airflow's own error is the one to report when Astro cannot say more
	}
	state := unavailableState(deployment.Status)
	if state == nil {
		return nil
	}
	return &UnavailableError{Name: i.Name, DeploymentID: i.Link.Deployment, State: state}
}

// The states an UnavailableError unwraps to, so a caller can branch with
// errors.Is.
var (
	ErrDeploymentHibernating = errors.New("deployment is hibernating")
	ErrDeploymentDeploying   = errors.New("deployment is deploying")
	ErrDeploymentUnhealthy   = errors.New("deployment is unhealthy")
	// ErrAirflowUnavailable is a Deployment Astro reports healthy whose Airflow
	// is not answering yet, as it is for a while after a wake-up or deploy.
	ErrAirflowUnavailable = errors.New("airflow is not answering")
)

// unavailableState is why a Deployment in status is not answering, or nil for
// a status Astro itself does not know.
func unavailableState(status astrov1.DeploymentStatus) error {
	switch status {
	case astrov1.DeploymentStatusHIBERNATING:
		return ErrDeploymentHibernating
	case astrov1.DeploymentStatusCREATING, astrov1.DeploymentStatusDEPLOYING:
		return ErrDeploymentDeploying
	case astrov1.DeploymentStatusUNHEALTHY:
		return ErrDeploymentUnhealthy
	case astrov1.DeploymentStatusHEALTHY:
		return ErrAirflowUnavailable
	case astrov1.DeploymentStatusUNKNOWN:
	}
	return nil
}

// UnavailableError is an Astro Deployment whose Airflow does not answer
// because of the state Astro reports it in. State is one of the sentinels
// above.
type UnavailableError struct {
	Name         string
	DeploymentID string
	State        error
}

func (e *UnavailableError) Unwrap() error { return e.State }

func (e *UnavailableError) Error() string {
	switch e.State {
	case ErrDeploymentHibernating:
		return fmt.Sprintf("Astro Deployment %s (%q) is hibernating, so its Airflow is not answering — wake it with astro deployment wake-up %s, which takes about a minute",
			e.DeploymentID, e.Name, e.DeploymentID)
	case ErrDeploymentDeploying:
		return fmt.Sprintf("Astro Deployment %s (%q) is still deploying, so its Airflow is not answering yet — try again in a minute or two",
			e.DeploymentID, e.Name)
	case ErrAirflowUnavailable:
		return fmt.Sprintf("Astro Deployment %s (%q) reports healthy, but its Airflow is not answering yet — it may still be starting after a wake-up or deploy, so try again in a minute",
			e.DeploymentID, e.Name)
	}
	return fmt.Sprintf("Astro reports Deployment %s (%q) as unhealthy, so its Airflow is not answering — see astro deployment inspect %s",
		e.DeploymentID, e.Name, e.DeploymentID)
}

// bearer puts the identity astrosession resolved on one request, replacing the
// header the client's own editor set from the login context. Set rather than
// Add: the generated client appends, and two Authorization headers is a 401
// with nothing to read.
func bearer(token string) astrov1.RequestEditorFn {
	// The token as BearerFor gave it, a stored one with its scheme or
	// ASTRO_API_TOKEN as set, read by the one rule, so the scheme goes out
	// once whichever it was.
	value := "Bearer " + astrosession.Credential(token)
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", value)
		return nil
	}
}

// deploymentOutage names what the control plane said. Each status has its own
// fix, and a status with none still carries Astro's own words rather than a
// bare number.
func deploymentOutage(domain string, i instances.Instance, deploymentID string, resp *astrov1.GetDeploymentResponse) error {
	name := i.Name
	status := 0
	if resp.HTTPResponse != nil {
		status = resp.HTTPResponse.StatusCode
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w (looking up %q)", astrosession.Rejected(domain), name)
	case http.StatusForbidden:
		return fmt.Errorf("you do not have access to Astro Deployment %s (%q) — ask a workspace admin to grant it", deploymentID, name)
	case http.StatusNotFound:
		if i.Source == instances.SourceDeploymentID {
			return fmt.Errorf("Astro Deployment %s does not exist", deploymentID)
		}
		return fmt.Errorf("Astro Deployment %s (%q) does not exist — check the deployment on this link in pyproject.toml", deploymentID, name)
	}
	detail := ""
	if resp.HTTPResponse != nil {
		// A 200 or 204 that did not decode leaves NormalizeAPIError with
		// nothing to report, so there is no detail to add — only a status that
		// was fine and a body that was not the Deployment.
		if apiErr := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); apiErr != nil {
			detail = ": " + apiErr.Error()
		}
	}
	return fmt.Errorf("Astro could not look up Deployment %s (%q)%s", deploymentID, name, detail)
}
