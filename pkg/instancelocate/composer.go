// Package instancelocate turns a Cloud Composer link's coordinates into an
// Airflow address.
//
// A link carries coordinates, not an address: it names an environment, and the
// environment's Airflow URI comes from the Composer API. pkg/instances cannot
// answer that for itself, so it takes a Locator, and this is the Composer half
// of one.
//
// # Why only Composer
//
// The astro half of the same job reads the control plane through the CLI's
// generated client and its login context, both of which live under internal/
// and cannot cross a module boundary. It stays in the CLI. A consumer with its
// own Deployment lookup — Astro Desktop has one, over its own platform client
// — needs exactly this half, and copying it was the alternative.
//
// So this exposes the lookup rather than a whole Locator: each consumer owns
// its own BaseURL switch and reaches here for the Composer case. That also
// puts the credential chain in the consumer's hands, which matters because the
// same chain has to answer the lookup and the Airflow calls after it. A run
// that finds an environment it then cannot talk to is the failure that
// separating them causes.
//
// Nothing here prints. Every failure is a named outage with the fix in the
// sentence.
package instancelocate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/astronomer/astro-cli/pkg/googleauth"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// lookupTimeout bounds one lookup. It is a single small GET against a cloud
// API, and a caller should not sit on it.
const lookupTimeout = 30 * time.Second

// Options are the seams the lookup resolves through.
//
// The zero value asks Google's own chain and the real Composer API, which is
// what a real run wants; a test supplies its own so it can answer without an
// account.
type Options struct {
	// Google is the credential chain the lookup runs under.
	//
	// It is pkg/googleauth's own Options rather than a restatement of its
	// fields, so a caller builds ONE value and hands it to both
	// googleauth.Provider and this lookup. That is what makes the invariant
	// structural instead of a rule to remember: the two halves of one
	// operation cannot end up on different chains, because there is only one
	// chain to be on. A field added there — an impersonation subject, a scope,
	// a quota project — reaches this half automatically.
	Google googleauth.Options
	// HTTPClient carries the lookup. nil uses one with lookupTimeout.
	HTTPClient *http.Client
	// Endpoint is the Composer API base URL. Empty uses the public one.
	Endpoint string
}

func (o Options) client() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: lookupTimeout}
}

func (o Options) endpoint() string {
	if o.Endpoint != "" {
		return o.Endpoint
	}
	return composerAPI
}

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

// ComposerBaseURL reads the environment's Airflow URI from the Composer API,
// under the same Application Default Credentials the Airflow calls will carry.
// Asking with the credentials that are about to be used means a machine whose
// ADC is missing or refused hears about it here, once, rather than as a 403
// from Airflow with nothing to say about Google.
//
// Only a Composer instance belongs here; every other kind either carries its
// own URL or is not addressed by one. Routing one of those here is refused by
// name rather than answered, because the alternative is advice about editing a
// [tool.astro.targets.composer] section on a link that has none.
func ComposerBaseURL(ctx context.Context, i instances.Instance, o Options) (string, error) {
	if i.Kind != instances.KindComposer {
		return "", fmt.Errorf("%q is a %s deployment, not a Composer environment: only a Composer link has an address to look up here", i.Name, i.Kind)
	}
	at, err := composerCoordinatesOf(i)
	if err != nil {
		return "", err
	}
	token, err := o.Google.ResolveToken(ctx)
	if err != nil {
		return "", err
	}

	target := fmt.Sprintf("%s/v1/projects/%s/locations/%s/environments/%s",
		o.endpoint(), url.PathEscape(at.project), url.PathEscape(at.location), url.PathEscape(at.environment))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := o.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach the Composer API to look up %q — check your connection: %w", i.Name, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxComposerBody))
	if err != nil {
		return "", fmt.Errorf("read the Composer API's answer for %q: %w", i.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", composerOutage(ctx, o, i.Name, at, resp.StatusCode, body)
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
func composerOutage(ctx context.Context, o Options, name string, at composerCoordinates, status int, body []byte) error {
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("Google rejected the credentials for %q — refresh them with gcloud auth application-default login", name)
	case http.StatusForbidden:
		msg := fmt.Sprintf("not allowed to read Composer environment %s in project %s — the account needs roles/composer.user",
			at.environment, at.project)
		// A sentence, not a laid-out line. This module answers a GUI as well
		// as a CLI, and a hard-wrapped indent is presentation the caller owns.
		if advice := googleauth.AccountAdvice(o.Google.ResolveAccount(ctx)); advice != "" {
			msg += ". " + advice
		}
		return errors.New(msg)
	case http.StatusNotFound:
		return fmt.Errorf("Composer environment %s does not exist in project %s, location %s — check the link's environment and [tool.astro.targets.composer] in pyproject.toml",
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
