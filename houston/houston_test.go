package houston

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

func TestNewHoustonClient(t *testing.T) {
	client := newInternalClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new houston Client")
}

func TestErrAuthTokenRefreshFailed(t *testing.T) {
	testUtil.InitTestConfig()
	mockResponse := &Response{
		Data: ResponseData{},
		Errors: []Error{
			{
				Message: ErrAuthTokenRefreshFailed.Error(),
				Name:    "",
			},
		},
	}
	jsonResponse, _ := json.Marshal(mockResponse)

	t.Run("Test ErrAuthTokenRefreshFailed error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		doOpts := httputil.DoOptions{
			Headers: map[string]string{
				"Accept": "application/json",
			},
		}
		houstonClient := &Client{HTTPClient: client}
		resp, err := houstonClient.Do(doOpts)

		assert.Contains(t, err.Error(), ErrVerboseInaptPermissions.Error())
		assert.Nil(t, resp)
	})
}
