package auth

import (
	"github.com/astronomer/astro-cli/config"
)

// higher order functions to facilate writing unit test cases
type (
	RequestToken             func(authConfig AuthConfig, verifier, code string) (Result, error)
	RequestUserInfo          func(authConfig AuthConfig, accessToken string) (UserInfo, error)
	AuthorizeCallbackHandler func() (string, error)
)

type Authenticator struct {
	userInfoRequester RequestUserInfo
	tokenRequester    RequestToken
	callbackHandler   AuthorizeCallbackHandler
}

type UserInfo struct {
	Email      string `json:"email"`
	Picture    string `json:"picture"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
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

type Result struct {
	RefreshToken string
	AccessToken  string
	ExpiresIn    int64
	UserEmail    string
}

type CallbackMessage struct {
	authorizationCode string
	errorMessage      string
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
