package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestGetAppConfig(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockAppConfig := &AppConfig{
		Version:                "1.0.5",
		BaseDomain:             "localdev.me",
		SMTPConfigured:         false,
		ManualReleaseNames:     false,
		ConfigureDagDeployment: false,
		NfsMountDagDeployment:  false,
		HardDeleteDeployment:   false,
		ManualNamespaceNames:   true,
		TriggererEnabled:       true,
		Flags: FeatureFlags{
			TriggererEnabled:     true,
			ManualNamespaceNames: true,
		},
	}
	mockResponse := Response{
		Data: ResponseData{
			GetAppConfig: mockAppConfig,
		},
	}
	jsonResponse, jsonErr := json.Marshal(mockResponse)
	assert.NoError(t, jsonErr)

	t.Run("success", func(t *testing.T) {
		countCalls := 0
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			countCalls++
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		config, err := api.GetAppConfig()
		assert.NoError(t, err)
		assert.Equal(t, config, mockAppConfig)

		config, err = api.GetAppConfig()
		assert.NoError(t, err)
		assert.Equal(t, config, mockAppConfig)

		assert.Equal(t, 1, countCalls)
	})

	t.Run("error", func(t *testing.T) {
		countCalls := 0
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			countCalls++
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		// reset the local variables
		appConfig = nil
		appConfigErr = nil

		config, err := api.GetAppConfig()
		assert.Contains(t, err.Error(), "Internal Server Error")
		assert.Nil(t, config)

		config, err = api.GetAppConfig()
		assert.Contains(t, err.Error(), "Internal Server Error")
		assert.Nil(t, config)

		assert.Equal(t, 1, countCalls)
	})

	t.Run("unavailable fields error", func(t *testing.T) {
		// reset the local variables
		appConfig = nil
		appConfigErr = nil

		response := `{"errors": [{"message": "Cannot query field \"triggererEnabled\" on type AppConfig."}]}`
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString(response)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.GetAppConfig()
		assert.EqualError(t, err, ErrFieldsNotAvailable{}.Error())
	})

	t.Run("method not available", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"
		houstonMethodAvailabilityByVersion["GetAppConfig"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.GetAppConfig()
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"GetAppConfig"})
	})
}

func TestGetAvailableNamespaces(t *testing.T) {
	testUtil.InitTestConfig("software")

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
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
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
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.GetAvailableNamespaces()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})

	t.Run("method not available", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"
		houstonMethodAvailabilityByVersion["GetAvailableNamespaces"] = VersionRestrictions{GTE: "0.29.0"}

		_, err := api.GetAvailableNamespaces()
		assert.ErrorIs(t, err, ErrMethodNotImplemented{"GetAvailableNamespaces"})
	})
}

func TestGetPlatformVersion(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockNamespaces := &Response{
		Data: ResponseData{
			GetAppConfig: &AppConfig{Version: "0.30.0"},
		},
	}
	jsonResponse, err := json.Marshal(mockNamespaces)
	assert.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)
		version = ""
		platformVersion, err := api.GetPlatformVersion()
		assert.NoError(t, err)
		assert.Equal(t, platformVersion, mockNamespaces.Data.GetAppConfig.Version)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)
		version = ""
		_, err := api.GetAvailableNamespaces()
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
