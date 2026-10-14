package instances

import (
	"context"
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The env vars a bare --url target reads its credential from. --url declares
// nothing and writes nothing, so the environment is the only place a
// credential for it can come from.
const (
	EnvToken    = "ASTRO_AIRFLOW_TOKEN" //nolint:gosec // the name of a variable, not a credential
	EnvUsername = "ASTRO_AIRFLOW_USERNAME"
	EnvPassword = "ASTRO_AIRFLOW_PASSWORD" //nolint:gosec // the name of a variable, not a credential
)

// EnvAPIToken is the Astro API token CI uses instead of a login session.
const EnvAPIToken = "ASTRO_API_TOKEN" //nolint:gosec // the name of a variable, not a credential

// credentials builds the credential source for an instance, and the refresh
// hook to install with it (nil when the credential cannot go stale mid-run).
// baseURL is the resolved location, which the local mint needs.
func credentials(ctx context.Context, i Instance, baseURL string, d Deps) (airflowapi.CredentialSource, func(context.Context) error, error) {
	if i.Kind == KindLocal {
		return localCredentials(i, baseURL, d)
	}
	if i.Source == SourceURL {
		return urlCredentials(baseURL, d)
	}
	switch i.Link.Auth.Method {
	case manifest.AuthNone:
		return nil, nil, nil
	case manifest.AuthAstro:
		return astroCredentials(d), nil, nil
	case manifest.AuthToken:
		token, err := d.envValue(i.Name, i.Link.Auth.TokenEnv)
		if err != nil {
			return nil, nil, err
		}
		return airflowapi.BearerToken(token), nil, nil
	case manifest.AuthBasic:
		username, err := d.envValue(i.Name, i.Link.Auth.UsernameEnv)
		if err != nil {
			return nil, nil, err
		}
		password, err := d.envValue(i.Name, i.Link.Auth.PasswordEnv)
		if err != nil {
			return nil, nil, err
		}
		return airflowapi.BasicAuth(username, password), nil, nil
	case manifest.AuthGoogle:
		return d.viaProvider(ctx, i, baseURL, manifest.AuthGoogle)
	case manifest.AuthAirflowToken:
		return airflowTokenCredentials(i, baseURL, d)
	case manifest.AuthExec:
		return execCredentials(i)
	case manifest.AuthAWS:
		// Not a credential on an HTTP request at all, so it never reaches
		// here: Transport dispatches on the method before a credential is
		// built. Kept as a refusal rather than a fallthrough so the two paths
		// cannot quietly disagree about which door a method uses.
		return nil, nil, fmt.Errorf("deployment %q proves itself to the AWS API rather than to an Airflow URL", i.Name)
	}
	return nil, nil, fmt.Errorf("deployment %q declares no auth method", i.Name)
}

// errLoggedOut reports a machine with neither a session nor the CI token. Both
// ways of having no session say the same thing, because the fix is the same.
var errLoggedOut = errors.New("you are not logged in — log in with astro login, or set " + EnvAPIToken)

// astroCredentials proves the caller with the Astro session. ASTRO_API_TOKEN
// wins when it is set, because that is how CI supplies an identity with no
// login on the machine at all.
//
// A session that cannot be read is a named outage, never a stack trace — the
// posture internal/emenv holds for the same session: the message says what
// happened and what to do about it. Which login the session reads, and whether
// a stale one is refreshed, is the caller's choice behind Deps.Session.
func astroCredentials(d Deps) airflowapi.CredentialSource {
	return func(ctx context.Context) (string, string, error) {
		// Read as airflowapi.BearerCredential reads it, the rule the CLI's
		// own session reader uses: a variable holding only the scheme
		// ("Bearer ") is no token, and the session answers instead.
		if token, ok := d.credentialEnv(EnvAPIToken); ok && airflowapi.BearerCredential(token) != "" {
			return airflowapi.BearerToken(token)(ctx)
		}
		if d.Session == nil {
			return "", "", errLoggedOut
		}
		token, err := d.Session(ctx)
		if err != nil {
			return "", "", err
		}
		if airflowapi.BearerCredential(token) == "" {
			return "", "", errLoggedOut
		}
		return airflowapi.BearerToken(token)(ctx)
	}
}

// urlCredentials reads the credential for a --url target from the environment.
// Nothing set means nothing sent: an open dev server is a real case, and
// guessing a credential would only turn a clear 401 into a confusing one.
//
// A username and password are exchanged rather than sent. An Airflow 3 takes
// neither on an API call — it wants the JWT its own /auth/token mints for them
// — while an Airflow 2 takes them as basic auth, and which one is behind a URL
// is discovered by asking: TokenMinter posts to /auth/token and falls back to
// basic auth on the 404 or 405 an Airflow with nothing to mint at answers. That
// is the exchange `astro api airflow --url` and a local Airflow already make.
func urlCredentials(baseURL string, d Deps) (airflowapi.CredentialSource, func(context.Context) error, error) {
	if token, ok := d.credentialEnv(EnvToken); ok {
		return airflowapi.BearerToken(token), nil, nil
	}
	username, hasUser := d.credentialEnv(EnvUsername)
	password, hasPassword := d.credentialEnv(EnvPassword)
	switch {
	case hasUser && hasPassword:
		minter, err := airflowapi.NewTokenMinter(baseURL, username, password, d.httpOptions()...)
		if err != nil {
			return nil, nil, err
		}
		source := func(ctx context.Context) (string, string, error) {
			scheme, value, err := minter.Credentials(ctx)
			if err != nil {
				return "", "", fmt.Errorf("%s could not exchange %s and %s for a token at its %s: %w",
					baseURL, EnvUsername, EnvPassword, airflowTokenPath, err)
			}
			return scheme, value, nil
		}
		return source, minter.Refresh, nil
	case hasUser != hasPassword:
		return nil, nil, fmt.Errorf("%s and %s go together: set both, or neither and use %s instead", EnvUsername, EnvPassword, EnvToken)
	}
	return nil, nil, nil
}

// envValue reads a credential the manifest named by env var — always a name,
// because the manifest requires every credential field its method takes and
// refuses an empty one. A name the machine has no value for is reported the way
// the env machinery reports any missing value: the variable, and the command
// that sets it.
func (d Deps) envValue(deployment, name string) (string, error) {
	value, ok := d.credentialEnv(name)
	if !ok {
		// "variable" is localenv.Noun(localenv.KindEnv). It is spelled out
		// rather than called because this is a separate module and cannot
		// import internal/; the one definition does not reach here, so a
		// rename of that noun has to update this literal too.
		return "", fmt.Errorf("deployment %q needs the env var %s, which is not set on this machine.\n      provide it:  astro local env variable set %s --project", deployment, name, name)
	}
	return value, nil
}

// credentialEnv reads an env var, counting an empty value as unset: an
// exported-but-empty variable is someone's half-finished setup, and sending an
// empty credential would only turn that into an unexplained 401.
func (d Deps) credentialEnv(name string) (string, bool) {
	value, ok := d.lookupEnv(name)
	return value, ok && value != ""
}
