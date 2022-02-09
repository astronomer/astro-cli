package houston

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestAuthenticateWithBasicAuth(t *testing.T) {
	testUtil.InitTestConfig()

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
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		token, err := api.AuthenticateWithBasicAuth("username", "password")
		assert.NoError(t, err)
		assert.Equal(t, token, mockToken.Data.CreateToken.Token.Value)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.AuthenticateWithBasicAuth("username", "password")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}

func TestGetAuthConfig(t *testing.T) {
	testUtil.InitTestConfig()

	mockAuthConfig := &Response{
		Data: ResponseData{
			GetAuthConfig: &AuthConfig{
				LocalEnabled: true,
				PublicSignup: true,
			},
		},
	}
	jsonResponse, err := json.Marshal(mockAuthConfig)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		authConfig, err := api.GetAuthConfig()
		assert.NoError(t, err)
		assert.Equal(t, authConfig, mockAuthConfig.Data.GetAuthConfig)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       ioutil.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.GetAuthConfig()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
