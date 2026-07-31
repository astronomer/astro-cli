package airflowapi

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// airflowAuthPath is where an Airflow that mints its own tokens does it. It
// sits at the server root, below any API version.
const airflowAuthPath = "/auth/token"

// mintFields is one auth manager's idea of a credential pair. Airflow 3 serves
// /auth/token from the deployment's auth manager, and the managers disagree
// about what a credential is called: FAB and the simple auth manager take a
// username and password, Keycloak an OAuth client id and secret. The endpoint,
// the method, and the answer are identical — the field names differ, and so
// does what an Airflow serving no /auth/token at all can be sent instead.
//
// The shapes are the ones the Keycloak auth manager's own documentation
// publishes: a password grant is username and password, with grant_type
// optional because it is the default, and a client-credentials grant is
// grant_type, client_id, and client_secret. Both go as JSON.
type mintFields struct {
	id     string
	secret string
	// grant is the OAuth grant_type to send, empty to send none. The password
	// grant leaves it out because every auth manager defaults to it and the
	// managers that are not Keycloak have never heard of the field.
	grant string
	// basic says this pair can go on the request itself when nothing mints. A
	// username and password can; OAuth client credentials cannot, and
	// pretending otherwise would send a client secret as a password to a
	// server that never asked for one.
	basic bool
}

var (
	passwordFields = mintFields{id: "username", secret: "password", basic: true}
	clientFields   = mintFields{id: "client_id", secret: "client_secret", grant: "client_credentials"}
)

// TokenMinter turns a credential pair into the credential an instance accepts,
// and holds it for the run. Airflow 3 mints a short-lived JWT at /auth/token;
// an Airflow that does not serve that endpoint takes a username and password
// directly, so a password minter falls back to basic auth. Which one happens
// is discovered by asking, not decided from a version.
//
// The pair to wire into a transport is Credentials and Refresh:
//
//	client := airflowapi.DefaultHTTPClient()
//	minter, err := airflowapi.NewTokenMinter(url, user, pass,
//		airflowapi.WithHTTPClient(client))
//	transport, err := airflowapi.NewHTTPTransport(url,
//		airflowapi.WithHTTPClient(client),
//		airflowapi.WithCredentials(minter.Credentials),
//		airflowapi.WithRefresh(minter.Refresh))
//
// Give both the same HTTP settings. The mint call is a request to the same
// server as everything after it, so a custom CA, a proxy, or a timeout that
// reaches only one of them is a mint that succeeds where the API calls fail,
// or the reverse.
//
// Nothing is written to disk: a minted token lives in memory for the run.
type TokenMinter struct {
	transport *HTTPTransport
	fields    mintFields
	id        string
	secret    string

	mu sync.Mutex
	// held says the credential below was worked out, which an empty scheme
	// does not: an Airflow with no auth at all settles on sending nothing.
	held   bool
	scheme string
	value  string
}

// NewTokenMinter builds a minter against an Airflow base URL. It takes the
// same options as NewHTTPTransport and they configure the mint call itself —
// WithHTTPClient is the one that applies, since minting is what produces the
// credentials.
//
// An empty username and password ask for a token anyway: an Airflow 3 in
// all-admins mode mints one for whoever asks.
func NewTokenMinter(baseURL, username, password string, opts ...HTTPOption) (*TokenMinter, error) {
	return newMinter(baseURL, passwordFields, username, password, opts...)
}

// NewClientCredentialsMinter builds a minter that exchanges an OAuth client id
// and secret at the same /auth/token endpoint, which is what Airflow 3 under
// the Keycloak auth manager accepts. An Airflow that does not serve the
// endpoint is a named failure rather than a fallback: client credentials mean
// an auth manager that mints, so an Airflow with nothing to mint at is
// misconfigured or is not the instance the caller thinks it is.
//
// The request matches the example in the Keycloak auth manager's own
// documentation. It has not been run against a live Keycloak deployment.
func NewClientCredentialsMinter(baseURL, clientID, clientSecret string, opts ...HTTPOption) (*TokenMinter, error) {
	return newMinter(baseURL, clientFields, clientID, clientSecret, opts...)
}

func newMinter(baseURL string, fields mintFields, id, secret string, opts ...HTTPOption) (*TokenMinter, error) {
	transport, err := NewHTTPTransport(baseURL, opts...)
	if err != nil {
		return nil, err
	}
	return &TokenMinter{transport: transport, fields: fields, id: id, secret: secret}, nil
}

// Credentials is a CredentialSource: it mints on first use and hands back the
// held credential after that.
//
// The lock is held across the mint, which Client.detect goes out of its way not
// to do. It is tolerated because a second caller arriving mid-mint wants
// exactly the token being minted and would only mint the same one again; the
// cost is that such a caller cannot give up on its own context. Worth
// revisiting if a caller ever fans out requests through one minter, which
// nothing here does.
func (m *TokenMinter) Credentials(ctx context.Context) (scheme, value string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held {
		return m.scheme, m.value, nil
	}
	scheme, value, err = m.mint(ctx)
	if err != nil {
		return "", "", err
	}
	m.held, m.scheme, m.value = true, scheme, value
	return scheme, value, nil
}

// Refresh drops the held credential so the next request mints again. It is
// the hook for WithRefresh: an Airflow 3 token is short-lived, and a 401
// halfway through a long command is what its expiry looks like.
func (m *TokenMinter) Refresh(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.held, m.scheme, m.value = false, "", ""
	return nil
}

func (m *TokenMinter) mint(ctx context.Context) (scheme, value string, err error) {
	req := m.mintRequest()
	resp, err := m.transport.Do(ctx, req)
	if err != nil {
		return "", "", err
	}
	// No /auth/token at all means an Airflow that never mints, so the
	// credentials go on the request itself — or nothing does, if there are
	// none. Only these two statuses say that: a proxy answering 403 here is a
	// refusal, and reading it as "mint elsewhere" would replace a clear error
	// with a confusing one.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		if !m.fields.basic {
			return "", "", fmt.Errorf("this airflow serves no %s to exchange %s for a token", airflowAuthPath, m.fields.id)
		}
		if m.id == "" && m.secret == "" {
			return "", "", nil
		}
		return basicScheme, basicValue(m.id, m.secret), nil
	}
	if err := statusError(req.Method, airflowAuthPath, resp); err != nil {
		return "", "", err
	}
	var minted struct {
		AccessToken string `json:"access_token"`
	}
	if err := resp.Decode(&minted); err != nil {
		return "", "", err
	}
	if minted.AccessToken == "" {
		return "", "", fmt.Errorf("airflow answered %s without an access_token", airflowAuthPath)
	}
	return bearerScheme, minted.AccessToken, nil
}

// mintRequest posts the credentials under the field names this minter's auth
// manager expects, or asks for a token with no body at all when there are none
// — the shape an Airflow 3 in all-admins mode answers.
func (m *TokenMinter) mintRequest() Request {
	if m.id == "" && m.secret == "" {
		return Request{Method: http.MethodGet, Path: airflowAuthPath}
	}
	body := map[string]string{m.fields.id: m.id, m.fields.secret: m.secret}
	if m.fields.grant != "" {
		body["grant_type"] = m.fields.grant
	}
	return Request{Method: http.MethodPost, Path: airflowAuthPath, Body: body}
}
