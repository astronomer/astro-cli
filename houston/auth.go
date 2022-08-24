package houston

import (
	"encoding/json"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// BasicAuthRequest - properties for basic authentication request
type BasicAuthRequest struct {
	Username string          `json:"identity"`
	Password string          `json:"password"`
	Ctx      *config.Context `json:"-"`
}

var (
	AuthConfigGetRequest = `
	query GetAuthConfig($redirect: String) {
		authConfig(redirect: $redirect) {
			localEnabled
			publicSignup
			initialSignup
			providers {
				name
        		displayName
				url
      		}
		}
	}`

	// nolint:gosec
	TokenBasicCreateRequest = `
	mutation createBasicToken($identity: String, $password: String!) {
		createToken(identity: $identity, password: $password) {
			user {
				id
				fullName
				username
				status
				createdAt
				updatedAt
			}
			token {
				value
			}
		}
	}`
)

// AuthenticateWithBasicAuth - authentiate to Houston using basic auth
func (h ClientImplementation) AuthenticateWithBasicAuth(request BasicAuthRequest) (string, error) {
	if err := h.ValidateAvailability(); err != nil {
		return "", err
	}

	req := Request{
		Query:     TokenBasicCreateRequest,
		Variables: request,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return "", handleAPIErr(err)
	}
	doOpts := httputil.DoOptions{
		Data: reqData,
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	resp, err := h.client.DoWithContext(doOpts, request.Ctx)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return resp.Data.CreateToken.Token.Value, nil
}

// GetAuthConfig - get authentication configuration
func (h ClientImplementation) GetAuthConfig(ctx *config.Context) (*AuthConfig, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

	acReq := Request{
		Query: AuthConfigGetRequest,
	}
	reqData, err := json.Marshal(acReq)
	if err != nil {
		return nil, handleAPIErr(err)
	}
	doOpts := httputil.DoOptions{
		Data: reqData,
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	acResp, err := h.client.DoWithContext(doOpts, ctx)
	if err != nil {
		return nil, handleAPIErr(err)
	}
	return acResp.Data.GetAuthConfig, nil
}
