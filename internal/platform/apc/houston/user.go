package houston

// CreateUserRequest - properties to create a user
type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserCreateRequest asks for the user only. createUser also mints a session
// token for the new user when it is active (houston-api
// ); the CLI has no use for a
// credential of someone else's, so it does not ask for one.
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
		}
	}`

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
