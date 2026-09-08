package instancelocate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/internal/instances/googleauth"
)

// The Composer API is called over plain REST rather than through
// google.golang.org/api. One GET for one field — the environment's Airflow URI
// — does not earn a generated client and everything it drags in; the ADC token
// the Airflow calls already carry is the same token this request needs.
const (
	// composerAPI is the public Composer API. Composer 3 and Composer 2 both
	// answer the v1 environments endpoint.
	composerAPI = "https://composer.googleapis.com"
	// maxComposerErrorBody bounds how much of an unexplained answer reaches
	// the message.
	maxComposerErrorBody = 300
	// maxComposerBody bounds the whole answer read off the wire. An
	// environment resource is small; anything larger is not one.
	maxComposerBody = 64 << 10
)

// The [tool.astro.targets.composer] fields that say where an environment
// lives. The link names the environment; these two name the project and region
// holding it, which is a fact about the backend rather than the link.
const (
	composerProjectKey  = "project"
	composerLocationKey = "location"
)

// composerEnvironment is the slice of the API's Environment resource this
// needs: where its Airflow answers.
type composerEnvironment struct {
	Config struct {
		AirflowURI string `json:"airflowUri"`
	} `json:"config"`
}

// composerCoordinates is where one Composer environment lives: its name from
// the link, and the project and region from the target section.
type composerCoordinates struct {
	environment string
	project     string
	location    string
}

// composerBaseURL reads the environment's Airflow URI from the Composer API,
// under the same Application Default Credentials the Airflow calls will carry.
// Asking with the credentials that are about to be used means a machine whose
// ADC is missing or refused hears about it here, once, rather than as a 403
// from Airflow with nothing to say about Google.
func (l *locator) composerBaseURL(ctx context.Context, i instances.Instance) (string, error) {
	at, err := composerCoordinatesOf(i)
	if err != nil {
		return "", err
	}
	token, err := l.googleToken(ctx)
	if err != nil {
		return "", err
	}

	target := fmt.Sprintf("%s/v1/projects/%s/locations/%s/environments/%s",
		l.composerEndpoint, url.PathEscape(at.project), url.PathEscape(at.location), url.PathEscape(at.environment))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach the Composer API to look up %q — check your connection: %w", i.Name, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxComposerBody))
	if err != nil {
		return "", fmt.Errorf("read the Composer API's answer for %q: %w", i.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", l.composerOutage(ctx, i.Name, at, resp.StatusCode, body)
	}

	var env composerEnvironment
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("decode the Composer environment %s: %w", at.environment, err)
	}
	if env.Config.AirflowURI == "" {
		return "", fmt.Errorf("Composer environment %s in %s reports no Airflow URI yet — it may still be starting", at.environment, at.location)
	}
	return env.Config.AirflowURI, nil
}

// composerCoordinatesOf gathers the two halves of a Composer address, naming
// whichever is missing. Both target fields are required: an environment name
// alone does not say which project or which region holds it.
func composerCoordinatesOf(i instances.Instance) (composerCoordinates, error) {
	at := composerCoordinates{environment: i.Link.Environment}
	if at.environment == "" {
		return at, fmt.Errorf("deployment %q names no Composer environment: set environment = '<environment name>' on the link", i.Name)
	}
	for _, field := range []struct {
		key  string
		into *string
	}{
		{composerProjectKey, &at.project},
		{composerLocationKey, &at.location},
	} {
		value, err := i.TargetString(field.key)
		if err != nil {
			return at, err
		}
		if value == "" {
			return at, fmt.Errorf("deployment %q needs %s = '<%s>' under [tool.astro.targets.composer]: an environment name alone does not say where it lives",
				i.Name, field.key, field.key)
		}
		*field.into = value
	}
	return at, nil
}

// composerOutage names what the Composer API refused and what to do about it.
//
// The 403 carries the extra sentence, because there are two ways to earn one
// and they have different fixes: the caller may lack roles/composer.user on
// the project, or the caller may be a service account whose email is too long
// for Composer's Airflow to register on its own. The second is identifiable —
// the credentials say who they speak for — so it is named when it applies
// rather than offered as a guess every time.
func (l *locator) composerOutage(ctx context.Context, name string, at composerCoordinates, status int, body []byte) error {
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("Google rejected the credentials for %q — refresh them with `gcloud auth application-default login`", name)
	case http.StatusForbidden:
		msg := fmt.Sprintf("not allowed to read Composer environment %s in project %s — the account needs roles/composer.user",
			at.environment, at.project)
		if advice := googleauth.AccountAdvice(l.googleAccount(ctx)); advice != "" {
			msg += ".\n      " + advice
		}
		return errors.New(msg)
	case http.StatusNotFound:
		return fmt.Errorf("Composer environment %s does not exist in project %s, location %s — check the link's `environment` and [tool.astro.targets.composer] in pyproject.toml",
			at.environment, at.project, at.location)
	}
	return fmt.Errorf("the Composer API answered %d %s looking up %q: %s",
		status, http.StatusText(status), name, truncate(string(body), maxComposerErrorBody))
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
