package auth

import (
	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/config"
)

// higher order functions to facilate writing unit test cases
type (
	OrgLookup                func(domain string) (string, error)
	RequestToken             func(authConfig astro.AuthConfig, verifier, code string) (Result, error)
	AuthorizeCallbackHandler func() (string, error)
)

type Authenticator struct {
	orgChecker      OrgLookup
	tokenRequester  RequestToken
	callbackHandler AuthorizeCallbackHandler
}

type postTokenResponse struct {
	AccessToken      string  `json:"access_token"`
	IDToken          string  `json:"id_token"`
	RefreshToken     string  `json:"refresh_token"`
	Scope            string  `json:"scope"`
	ExpiresIn        int64   `json:"expires_in"`
	TokenType        string  `json:"token_type"`
	Error            *string `json:"error,omitempty"`
	ErrorDescription string  `json:"error_description,omitempty"`
}

type orgLookupRequest struct {
	Email string `json:"email"`
}

type orgLookupResults struct {
	OrganizationIds []string
}

type Result struct {
	RefreshToken string
	AccessToken  string
	ExpiresIn    int64
	UserEmail    string
}

func (res Result) writeToContext(c *config.Context) error {
	err = c.SetContextKey("token", "Bearer "+res.AccessToken)
	if err != nil {
		return err
	}

	err = c.SetContextKey("refreshtoken", res.RefreshToken)
	if err != nil {
		return err
	}

	err = c.SetExpiresIn(res.ExpiresIn)
	if err != nil {
		return err
	}

	err = c.SetContextKey("user_email", res.UserEmail)
	if err != nil {
		return err
	}
	return nil
}
