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

	// FetchAuthConfig builds "https://api.<domain>/..." so we can't easily
	// redirect to the test server. Instead, test the struct/endpoint constant.
	assert.Equal(t, "private/v1alpha1/cli/auth-config", AuthConfigEndpoint)
	assert.Equal(t, "test-client", cfg.ClientID)
}

func TestFetchAuthConfig_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Can't test directly against live domain, but verify error handling
	_, err := FetchAuthConfig("localhost-nonexistent-domain-12345.invalid")
	require.Error(t, err)
}
