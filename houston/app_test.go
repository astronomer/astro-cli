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

func TestGetAppConfig(t *testing.T) {
	testUtil.InitTestConfig()

	mockAppConfig := &Response{
		Data: ResponseData{
			GetAppConfig: &AppConfig{
				TriggererEnabled: true,
			},
		},
	}
	jsonResponse, err := json.Marshal(mockAppConfig)
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

		appConfig, err := api.GetAppConfig()
		assert.NoError(t, err)
		assert.True(t, appConfig.TriggererEnabled)
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

		_, err := api.GetAppConfig()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("unavailable fields error", func(t *testing.T) {
		response := `{"errors": [{"message": "Cannot query field \"triggererEnabled\" on type AppConfig."}]}`
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       ioutil.NopCloser(bytes.NewBufferString(response)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.GetAppConfig()
		assert.EqualError(t, err, ErrFieldsNotAvailable{}.Error())
	})
}

func TestGetAvailableNamespaces(t *testing.T) {
	testUtil.InitTestConfig()

	mockNamespaces := &Response{
		Data: ResponseData{
			GetDeploymentNamespaces: []Namespace{
				{Name: "test1"},
				{Name: "test2"},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockNamespaces)
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

		namespaces, err := api.GetAvailableNamespaces()
		assert.NoError(t, err)
		assert.Equal(t, namespaces, mockNamespaces.Data.GetDeploymentNamespaces)
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

		_, err := api.GetAvailableNamespaces()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
