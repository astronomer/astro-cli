package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestNewHoustonClient() {
	client := newInternalClient(httputil.NewHTTPClient())
	s.NotNil(client, "Can't create new houston Client")
}

func (s *Suite) TestErrAuthTokenRefreshFailed() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockResponse := &Response{
		Data: ResponseData{},
		Errors: []Error{
			{
				Message: errAuthTokenRefreshFailedMsg,
				Name:    "",
			},
		},
	}
	jsonResponse, _ := json.Marshal(mockResponse)

	s.Run("Test ErrAuthTokenRefreshFailed error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		doOpts := &httputil.DoOptions{
			Headers: map[string]string{
				"Accept": "application/json",
			},
		}
		houstonClient := &Client{HTTPClient: client}
		resp, err := houstonClient.Do(doOpts)

		s.Contains(err.Error(), ErrVerboseInaptPermissions.Error())
		s.Nil(resp)
	})
}

func (s *Suite) TestNewHTTPClient() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	client := NewHTTPClient()
	s.NotNil(client)
}
