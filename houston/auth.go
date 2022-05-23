package houston

import (
	"encoding/json"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// AuthenticateWithBasicAuth - authentiate to Houston using basic auth
func (h ClientImplementation) AuthenticateWithBasicAuth(username, password string, ctx *config.Context) (string, error) {
	req := Request{
		Query:     TokenBasicCreateRequest,
		Variables: map[string]interface{}{"identity": username, "password": password},
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

	resp, err := h.client.DoWithContext(doOpts, ctx)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return resp.Data.CreateToken.Token.Value, nil
}

// GetAuthConfig - get authentication configuration
func (h ClientImplementation) GetAuthConfig(ctx *config.Context) (*AuthConfig, error) {
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
