package instancelocate

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/instances"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// astroBaseURL reads the Deployment's web server URL from the control plane.
// It is one call per command, made only when a command is about to talk to
// that Airflow.
//
// The identity is resolved before the call rather than after it because a
// machine with none gets a better sentence from us than from an API that was
// never reached. It is then put on the request, so ASTRO_API_TOKEN outranks
// whatever a stale login context holds — the same precedence the credential
// path uses, since the two must not disagree about who is calling.
func (l *locator) astroBaseURL(ctx context.Context, i instances.Instance) (string, error) {
	deploymentID := i.Link.Deployment
	if deploymentID == "" {
		return "", fmt.Errorf("deployment %q names no Deployment: set deployment = '<id>' on the link", i.Name)
	}
	if l.session == nil {
		return "", astrosession.ErrLoggedOut
	}
	token, err := l.session(ctx)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", astrosession.ErrLoggedOut
	}
	org, err := l.organization()
	if err != nil {
		return "", err
	}
	resp, err := l.deployments.GetDeploymentWithResponse(ctx, org, deploymentID, bearer(token))
	if err != nil {
		// The request never reached an answer, which on this path means the
		// machine could not get to Astro at all.
		return "", fmt.Errorf("could not reach Astro to look up %q — check your connection: %w", i.Name, err)
	}
	if resp.JSON200 == nil {
		return "", deploymentOutage(i.Name, deploymentID, resp)
	}
	url := resp.JSON200.WebServerAirflowApiUrl
	if url == "" {
		return "", fmt.Errorf("Astro Deployment %s (%q) has no Airflow API URL yet — it may still be starting", deploymentID, i.Name)
	}
	return url, nil
}

// bearer puts the identity astrosession resolved on one request, replacing the
// header the client's own editor set from the login context. Set rather than
// Add: the generated client appends, and two Authorization headers is a 401
// with nothing to read.
func bearer(token string) astrov1.RequestEditorFn {
	value := token
	if !strings.HasPrefix(value, "Bearer ") {
		value = "Bearer " + value
	}
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", value)
		return nil
	}
}

// deploymentOutage names what the control plane said. Each status has its own
// fix, and a status with none still carries Astro's own words rather than a
// bare number.
func deploymentOutage(name, deploymentID string, resp *astrov1.GetDeploymentResponse) error {
	status := 0
	if resp.HTTPResponse != nil {
		status = resp.HTTPResponse.StatusCode
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("your session expired — log in again with `astro login` (looking up %q)", name)
	case http.StatusForbidden:
		return fmt.Errorf("you do not have access to Astro Deployment %s (%q) — ask a workspace admin to grant it", deploymentID, name)
	case http.StatusNotFound:
		return fmt.Errorf("Astro Deployment %s (%q) does not exist — check the `deployment` on this link in pyproject.toml", deploymentID, name)
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
