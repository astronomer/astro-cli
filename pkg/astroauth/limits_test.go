package astroauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultClientHasATimeout(t *testing.T) {
	o := resolveOptions(nil)
	require.NotNil(t, o.httpClient)
	assert.Equal(t, requestTimeout, o.httpClient.Timeout)
	assert.Nil(t, o.httpClient.Transport, "a nil Transport reads http.DefaultTransport, where the CLI installs its socket routing")
}

func TestRefreshTokenRejectsAnOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Valid JSON, but past the cap: it must be refused, not decoded.
		_, _ = w.Write([]byte(`{"access_token":"` + strings.Repeat("a", maxResponseBytes) + `"}`))
	}))
	defer srv.Close()

	cfg := AuthConfig{ClientID: "c", DomainURL: srv.URL + "/"}
	_, err := RefreshToken(cfg, "old-refresh", WithHTTPClient(srv.Client()))
	require.ErrorIs(t, err, errResponseTooLarge)
}

// A token endpoint that redirects is refused, and nothing reaches the target:
// the form carries the refresh token.
func TestRefreshTokenDoesNotFollowARedirect(t *testing.T) {
	for _, code := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			reached := false
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				_, _ = w.Write([]byte(`{"access_token":"x"}`))
			}))
			defer target.Close()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/oauth/token", code)
			}))
			defer srv.Close()

			client := srv.Client()
			cfg := AuthConfig{ClientID: "c", DomainURL: srv.URL + "/"}
			_, err := RefreshToken(cfg, "old-refresh", WithHTTPClient(client))
			require.ErrorIs(t, err, errTokenRedirect)
			assert.False(t, reached, "the redirect target was sent the request")
			assert.Nil(t, client.CheckRedirect, "the caller's client was changed")
		})
	}
}

func TestExchangeCodeDoesNotFollowARedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/oauth/token", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	cfg := AuthConfig{ClientID: "c", DomainURL: srv.URL + "/"}
	_, err := ExchangeCode(cfg, "v", "code", WithHTTPClient(srv.Client()))
	require.ErrorIs(t, err, errTokenRedirect)
}
