package houston

// CreateUserRequest - properties to create a user
type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

var UserCreateRequest = `
	mutation CreateUser(
		$email: String!
		$password: String!
		$username: String
		$inviteToken: String
	){
		createUser(
			email: $email
			password: $password
			username: $username
			inviteToken: $inviteToken
		){
			user {
				id
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

// CreateUser - Send a request to create a user in the Houston API
func (h ClientImplementation) CreateUser(request CreateUserRequest) (*AuthUser, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
