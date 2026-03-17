package astroauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// UserInfo holds profile data from the OAuth userinfo endpoint.
type UserInfo struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	Picture    string `json:"picture"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
}

// FetchUserInfo retrieves the user's profile from the OAuth userinfo endpoint.
func FetchUserInfo(authCfg AuthConfig, accessToken string) (*UserInfo, error) {
	addr := authCfg.DomainURL + "userinfo"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, addr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("cannot decode userinfo: %w", err)
	}
	if info.Email == "" {
		return nil, fmt.Errorf("no email in userinfo response")
	}
	return &info, nil
}
