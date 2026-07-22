package astroauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchAuthConfig_Success(t *testing.T) {
	cfg := AuthConfig{
		ClientID:  "test-client",
		Audience:  "test-audience",
		DomainURL: "https://auth.example.com/",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/"+AuthConfigEndpoint, r.URL.Path)
		assert.Equal(t, "cli", r.Header.Get("X-Astro-Client-Identifier"))
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(cfg)
	}))
	defer srv.Close()

	got, err := FetchAuthConfig("example.io", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	assert.Equal(t, cfg, got)
}

func TestFetchAuthConfig_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := FetchAuthConfig("example.io", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	require.Error(t, err)
}
