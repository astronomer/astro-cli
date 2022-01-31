package houston

// CreateUser - Send a request to create a user in the Houston API
func (h ClientImplementation) CreateUser(email, password string) (*AuthUser, error) {
	req := Request{
		Query:     UserCreateRequest,
		Variables: map[string]interface{}{"email": email, "password": password},
	}

	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return resp.Data.CreateUser, nil
}
