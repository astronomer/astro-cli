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
