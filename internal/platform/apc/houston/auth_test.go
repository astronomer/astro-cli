package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestAuthenticateWithBasicAuth() {
	testUtil.InitTestConfig("software")

	ctx, err := config.GetCurrentContext()
	s.NoError(err)

	mockToken := &Response{
		Data: ResponseData{
			CreateToken: &AuthUser{
				Token: Token{
					Value: "testing-token",
				},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockToken)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		token, err := api.AuthenticateWithBasicAuth(BasicAuthRequest{"username", "password", &ctx})
		s.NoError(err)
		s.Equal(token, mockToken.Data.CreateToken.Token.Value)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.AuthenticateWithBasicAuth(BasicAuthRequest{"username", "password", &ctx})
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetAuthConfig() {
	testUtil.InitTestConfig("software")

	ctx, err := config.GetCurrentContext()
	s.NoError(err)

	mockAuthConfig := &Response{
		Data: ResponseData{
			GetAuthConfig: &AuthConfig{
				LocalEnabled: true,
				PublicSignup: true,
			},
		},
	}
	jsonResponse, err := json.Marshal(mockAuthConfig)
	s.NoError(err)

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		authConfig, err := api.GetAuthConfig(&ctx)
		s.NoError(err)
		s.Equal(authConfig, mockAuthConfig.Data.GetAuthConfig)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.GetAuthConfig(&ctx)
		s.Contains(err.Error(), "Internal Server Error")
	})
}
