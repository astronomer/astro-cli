package houston

// CreateUserRequest - properties to create a user
type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateUser - Send a request to create a user in the Houston API
func (h ClientImplementation) CreateUser(request CreateUserRequest) (*AuthUser, error) {
	req := Request{
		Query:     UserCreateRequest,
		Variables: request,
	}

	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.CreateUser, nil
}
