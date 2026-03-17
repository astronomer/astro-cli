package astroauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExchangeCode_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		r.ParseForm()
		assert.Equal(t, "test-client", r.Form.Get("client_id"))
		assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		assert.Equal(t, "test-code", r.Form.Get("code"))
		assert.Equal(t, "test-verifier", r.Form.Get("code_verifier"))

		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			ExpiresIn:    3600,
		})
	}))
	defer srv.Close()

	cfg := AuthConfig{
		ClientID:  "test-client",
		DomainURL: srv.URL + "/",
	}

	resp, err := ExchangeCode(cfg, "test-verifier", "test-code")
	require.NoError(t, err)
	assert.Equal(t, "access-123", resp.AccessToken)
	assert.Equal(t, "refresh-456", resp.RefreshToken)
	assert.Equal(t, int64(3600), resp.ExpiresIn)
}

func TestExchangeCode_Error(t *testing.T) {
	errStr := "invalid_grant"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(TokenResponse{
			Error:     &errStr,
			ErrorDesc: "The code has expired",
		})
	}))
	defer srv.Close()

	cfg := AuthConfig{ClientID: "c", DomainURL: srv.URL + "/"}
	_, err := ExchangeCode(cfg, "v", "code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "The code has expired")
}

func TestRefreshToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		assert.Equal(t, "old-refresh", r.Form.Get("refresh_token"))

		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    7200,
		})
	}))
	defer srv.Close()

	cfg := AuthConfig{ClientID: "c", DomainURL: srv.URL + "/"}
	resp, err := RefreshToken(cfg, "old-refresh")
	require.NoError(t, err)
	assert.Equal(t, "new-access", resp.AccessToken)
}

func TestIsTokenExpired(t *testing.T) {
	// Empty string → expired
	assert.True(t, IsTokenExpired("", 5*time.Minute))

	// Invalid format → expired
	assert.True(t, IsTokenExpired("not-a-date", 5*time.Minute))

	// Future time → not expired
	future := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	assert.False(t, IsTokenExpired(future, 5*time.Minute))

	// Past time → expired
	past := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	assert.True(t, IsTokenExpired(past, 5*time.Minute))

	// Within threshold → expired
	almostExpired := time.Now().Add(3 * time.Minute).Format(time.RFC3339)
	assert.True(t, IsTokenExpired(almostExpired, 5*time.Minute))
}
