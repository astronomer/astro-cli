package astroauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const AuthConfigEndpoint = "private/v1alpha1/cli/auth-config"

// AuthConfig holds the OAuth configuration fetched from an Astronomer domain.
type AuthConfig struct {
	ClientID  string `json:"clientId"`
	Audience  string `json:"audience"`
	DomainURL string `json:"domainUrl"`
}

// FetchAuthConfig retrieves OAuth configuration from an Astronomer domain.
// The domain should be just the base domain (e.g., "astronomer.io"), not a full URL.
func FetchAuthConfig(domain string) (AuthConfig, error) {
	addr := fmt.Sprintf("https://api.%s/%s", domain, AuthConfigEndpoint)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, addr, nil)
	if err != nil {
		return AuthConfig{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Astro-Client-Identifier", "cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return AuthConfig{}, fmt.Errorf("cannot reach %s: %w", domain, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return AuthConfig{}, fmt.Errorf("auth config request failed (status %d)", resp.StatusCode)
	}

	var cfg AuthConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return AuthConfig{}, fmt.Errorf("cannot decode auth config: %w", err)
	}
	return cfg, nil
}
