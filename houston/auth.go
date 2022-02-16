package houston

// AuthenticateWithBasicAuth - authentiate to Houston using basic auth
func (h ClientImplementation) AuthenticateWithBasicAuth(username, password string) (string, error) {
	req := Request{
		Query:     TokenBasicCreateRequest,
		Variables: map[string]interface{}{"identity": username, "password": password},
	}

	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return resp.Data.CreateToken.Token.Value, nil
}

// GetAuthConfig - get authentication configuration
func (h ClientImplementation) GetAuthConfig() (*AuthConfig, error) {
	acReq := Request{
		Query: AuthConfigGetRequest,
	}

	acResp, err := acReq.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}
	return acResp.Data.GetAuthConfig, nil
}
