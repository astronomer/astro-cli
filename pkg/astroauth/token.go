package astroauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CallbackAddr = "localhost:12345"
	RedirectURI  = "http://localhost:12345/callback"
)

// TokenResponse holds the result of a token exchange or refresh.
type TokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresIn    int64   `json:"expires_in"`
	Error        *string `json:"error,omitempty"`
	ErrorDesc    string  `json:"error_description,omitempty"`
}

// ExchangeCode exchanges an authorization code for tokens using PKCE.
func ExchangeCode(authCfg AuthConfig, verifier, code string) (*TokenResponse, error) {
	addr := authCfg.DomainURL + "oauth/token"
	data := url.Values{
		"client_id":     {authCfg.ClientID},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {RedirectURI},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, addr, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("cannot decode token response: %w", err)
	}
	if tokenResp.Error != nil {
		return nil, fmt.Errorf("token error: %s", tokenResp.ErrorDesc)
	}
	return &tokenResp, nil
}

// RefreshToken exchanges a refresh token for new tokens.
func RefreshToken(authCfg AuthConfig, refreshTok string) (*TokenResponse, error) {
	addr := authCfg.DomainURL + "oauth/token"
	data := url.Values{
		"client_id":     {authCfg.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshTok},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, addr, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("cannot decode refresh response: %w", err)
	}
	if tokenResp.Error != nil {
		return nil, fmt.Errorf("refresh error: %s", tokenResp.ErrorDesc)
	}
	return &tokenResp, nil
}

// IsTokenExpired checks if a stored expiration time string (RFC3339) is expired
// or within the given threshold of expiring.
func IsTokenExpired(expiresInStr string, threshold time.Duration) bool {
	if expiresInStr == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, expiresInStr)
	if err != nil {
		return true
	}
	return time.Now().Add(threshold).After(t)
}
