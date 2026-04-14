package auth

import (
	"github.com/astronomer/astro-cli/pkg/astroauth"
)

// higher order functions to facilate writing unit test cases
type (
	RequestToken             func(authConfig Config, verifier, code string) (Result, error)
	RequestUserInfo          func(authConfig Config, accessToken string) (UserInfo, error)
	AuthorizeCallbackHandler func() (string, error)
)

type Authenticator struct {
	userInfoRequester RequestUserInfo
	tokenRequester    RequestToken
	callbackHandler   AuthorizeCallbackHandler
}

// UserInfo is an alias for astroauth.UserInfo.
type UserInfo = astroauth.UserInfo

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
