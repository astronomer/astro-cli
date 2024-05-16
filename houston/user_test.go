package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCreateUser() {
	testUtil.InitTestConfig("software")

	mockResponse := &Response{
		Data: ResponseData{
			CreateUser: &AuthUser{
				User: User{
					ID: "user-id",
					Emails: []Email{
						{Address: "test@astronomer.com"},
					},
					Username: "username",
					Status:   "active",
				},
				Token: Token{
					Value: "test-token",
				},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
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

		response, err := api.CreateUser(CreateUserRequest{"email", "password"})
		s.NoError(err)
		s.Equal(response, mockResponse.Data.CreateUser)
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

		_, err := api.CreateUser(CreateUserRequest{"email", "password"})
		s.Contains(err.Error(), "Internal Server Error")
	})
}
