package houston

// AuthenticateWithBasicAuth - authentiate to Houston using basic auth
func (h HoustonClientImplementation) AuthenticateWithBasicAuth(username, password string) (string, error) {
	req := Request{
		Query:     TokenBasicCreateRequest,
		Variables: map[string]interface{}{"identity": username, "password": password},
	}

	resp, err := req.Do()
	if err != nil {
		return "", err
	}

	return resp.Data.CreateToken.Token.Value, nil
}

// GetAuthConfig - get authentication configuration
func (h HoustonClientImplementation) GetAuthConfig() (*AuthConfig, error) {
	acReq := Request{
		Query: AuthConfigGetRequest,
	}
	
	acResp, err := acReq.Do()
	if err != nil {
		return nil, err
	}
	return acResp.Data.GetAuthConfig, nil
}