package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestGetAppConfig() {
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
	s.NoError(jsonErr)

	s.Run("success", func() {
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

		config, err := api.GetAppConfig("")
		s.NoError(err)
		s.Equal(config, mockAppConfig)

		config, err = api.GetAppConfig("")
		s.NoError(err)
		s.Equal(config, mockAppConfig)

		s.Equal(2, countCalls)
	})

	s.Run("error", func() {
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

		config, err := api.GetAppConfig("")
		s.Contains(err.Error(), "Internal Server Error")
		s.Nil(config)

		config, err = api.GetAppConfig("")
		s.Contains(err.Error(), "Internal Server Error")
		s.Nil(config)

		s.Equal(2, countCalls)
	})

	s.Run("unavailable fields error", func() {
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

		_, err := api.GetAppConfig("")
		s.EqualError(err, ErrFieldsNotAvailable{}.Error())
	})
}

func (s *Suite) TestGetAvailableNamespaces() {
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

		namespaces, err := api.GetAvailableNamespaces(nil)
		s.NoError(err)
		s.Equal(namespaces, mockNamespaces.Data.GetDeploymentNamespaces)
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

		_, err := api.GetAvailableNamespaces(nil)
		s.Contains(err.Error(), "Internal Server Error")
	})
}

func (s *Suite) TestGetPlatformVersion() {
	testUtil.InitTestConfig("software")

	mockNamespaces := &Response{
		Data: ResponseData{
			GetAppConfig: &AppConfig{Version: "0.30.0"},
		},
	}
	jsonResponse, err := json.Marshal(mockNamespaces)
	s.NoError(err)

	s.Run("non empty version", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)
		version = "0.30.0"
		versionErr = nil
		resp, err := api.GetPlatformVersion(nil)
		s.NoError(err)
		s.Equal(version, resp)
	})

	s.Run("non empty version error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)
		version = ""
		versionErr = errMockHouston
		resp, err := api.GetPlatformVersion(nil)
		s.ErrorIs(err, errMockHouston)
		s.Equal(version, resp)
	})

	s.Run("success", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)
		version = ""
		versionErr = nil
		platformVersion, err := api.GetPlatformVersion(nil)
		s.NoError(err)
		s.Equal(platformVersion, mockNamespaces.Data.GetAppConfig.Version)
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
		version = ""
		versionErr = nil
		_, err := api.GetPlatformVersion(nil)
		s.Contains(err.Error(), "Internal Server Error")
	})
}
