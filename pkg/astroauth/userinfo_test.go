package astroauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchUserInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		json.NewEncoder(w).Encode(UserInfo{
			Email:   "user@example.com",
			Name:    "Test User",
			Picture: "https://example.com/pic.jpg",
		})
	}))
	defer srv.Close()

	cfg := AuthConfig{DomainURL: srv.URL + "/"}
	info, err := FetchUserInfo(cfg, "test-token")
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", info.Email)
	assert.Equal(t, "Test User", info.Name)
}

func TestFetchUserInfo_NoEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(UserInfo{Name: "No Email"})
	}))
	defer srv.Close()

	cfg := AuthConfig{DomainURL: srv.URL + "/"}
	_, err := FetchUserInfo(cfg, "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no email")
}
