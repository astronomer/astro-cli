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

// TokenMinter turns a username and password into the credential an instance
// accepts, and holds it for the run. Airflow 3 mints a short-lived JWT at
// /auth/token; an Airflow that does not serve that endpoint takes the
// username and password directly, so the minter falls back to basic auth.
// Which one happens is discovered by asking, not decided from a version.
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
	username  string
	password  string

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
	transport, err := NewHTTPTransport(baseURL, opts...)
	if err != nil {
		return nil, err
	}
	return &TokenMinter{transport: transport, username: username, password: password}, nil
}

// Credentials is a CredentialSource: it mints on first use and hands back the
// held credential after that.
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
		if m.username == "" && m.password == "" {
			return "", "", nil
		}
		return basicScheme, basicValue(m.username, m.password), nil
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

// mintRequest posts the credentials, or asks for a token with no body at all
// when there are none — the shape an Airflow 3 in all-admins mode answers.
func (m *TokenMinter) mintRequest() Request {
	if m.username == "" && m.password == "" {
		return Request{Method: http.MethodGet, Path: airflowAuthPath}
	}
	return Request{
		Method: http.MethodPost,
		Path:   airflowAuthPath,
		Body:   map[string]string{"username": m.username, "password": m.password},
	}
}
