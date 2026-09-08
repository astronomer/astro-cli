package instances

import (
	"context"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// airflowTokenPath is where the mint happens, spelled here only so a failure
// can say where it went.
const airflowTokenPath = "/auth/token" //nolint:gosec // the path a token is minted at, not a credential

// airflowTokenCredentials mints a token at the instance's own /auth/token with
// credentials the manifest named by env var.
//
// Which pair it sends is the manifest's choice, because it is the instance's
// auth manager that decides: Airflow 3 under FAB or the simple auth manager
// reads a username and password, under Keycloak a client id and secret. The
// manifest requires exactly one whole pair, so there is nothing to guess here
// — the pair that is declared is the pair that is sent.
//
// Nothing lands on disk. The token lives in memory for the run, and the
// refresh hook re-mints when a short-lived one expires mid-command.
func airflowTokenCredentials(i Instance, baseURL string, d Deps) (airflowapi.CredentialSource, func(context.Context) error, error) {
	auth := i.Link.Auth
	idEnv, secretEnv, newMinter := auth.UsernameEnv, auth.PasswordEnv, airflowapi.NewTokenMinter
	if auth.ClientIDEnv != "" {
		idEnv, secretEnv, newMinter = auth.ClientIDEnv, auth.ClientSecretEnv, airflowapi.NewClientCredentialsMinter
	}
	id, err := d.envValue(i.Name, idEnv)
	if err != nil {
		return nil, nil, err
	}
	secret, err := d.envValue(i.Name, secretEnv)
	if err != nil {
		return nil, nil, err
	}
	minter, err := newMinter(baseURL, id, secret, d.httpOptions()...)
	if err != nil {
		return nil, nil, err
	}
	// Named on the way out. A mint failure is one Airflow refusing one
	// exchange, and a reader looking at a bare 401 has no way to tell which
	// instance refused — the same reason a missing env var names its instance.
	source := func(ctx context.Context) (string, string, error) {
		scheme, value, err := minter.Credentials(ctx)
		if err != nil {
			return "", "", fmt.Errorf("deployment %q could not mint a token at its own %s: %w", i.Name, airflowTokenPath, err)
		}
		return scheme, value, nil
	}
	return source, minter.Refresh, nil
}
