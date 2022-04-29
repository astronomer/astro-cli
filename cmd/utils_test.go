package cmd

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	mocks "github.com/astronomer/astro-cli/houston/mocks"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

var mockDeploymentConfig = &houston.DeploymentConfig{
	AirflowVersions: []string{
		"2.1.0",
		"2.0.2",
		"2.0.0",
		"1.10.15",
		"1.10.14",
		"1.10.12",
		"1.10.10",
		"1.10.7",
		"1.10.5",
	},
}

func Test_prepareDefaultAirflowImageTag(t *testing.T) {
	testUtil.InitTestConfig()

	// prepare fake response from updates.astronomer.io
	okResponse := `{
  "version": "1.0",
  "available_releases": [
    {
      "version": "1.10.5",
      "level": "new_feature",
      "url": "https://github.com/astronomer/airflow/releases/tag/1.10.5-11",
      "release_date": "2020-10-05T20:03:00+00:00",
      "tags": [
        "1.10.5-alpine3.10-onbuild",
        "1.10.5-buster-onbuild",
        "1.10.5-alpine3.10",
        "1.10.5-buster"
      ],
      "channel": "stable"
    }
  ]
}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	httpClient := airflowversions.NewClient(client, true)

	// prepare fake response from houston
	api := new(mocks.ClientInterface)
	api.On("GetDeploymentConfig").Return(mockDeploymentConfig, nil)

	output := new(bytes.Buffer)

	airflowTests := []struct {
		airflowVersion   string
		expectedImageTag string
		expectedError    string
	}{
		{airflowVersion: "1.10.14", expectedImageTag: "1.10.14", expectedError: ""},
		{airflowVersion: "1.10.15", expectedImageTag: "1.10.15", expectedError: ""},
		{airflowVersion: "", expectedImageTag: "1.10.5-buster-onbuild", expectedError: ""},
		{airflowVersion: "2.0.2", expectedImageTag: "2.0.2", expectedError: ""},
		{airflowVersion: "9.9.9", expectedImageTag: "", expectedError: "Unsupported Airflow Version specified. Please choose from: 2.1.0, 2.0.2, 2.0.0, 1.10.15, 1.10.14, 1.10.12, 1.10.10, 1.10.7, 1.10.5 \n"},
	}
	for _, tt := range airflowTests {
		defaultTag, err := prepareDefaultAirflowImageTag(tt.airflowVersion, "", httpClient, api, output)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, tt.expectedImageTag, defaultTag)
	}
}

func Test_prepareDefaultImageTagRuntime(t *testing.T) {
	runtimeResponse := `
{
   "runtimeVersions":{
      "3.0.0":{
         "metadata":{
            "airflowVersion":"2.1.1",
            "channel":"stable",
            "releaseDate":"2021-08-12"
         },
         "migrations":{
            "airflowDatabase":false
         }
      },
      "3.0.1":{
         "metadata":{
            "airflowVersion":"2.1.1",
            "channel":"stable",
            "releaseDate":"2021-08-31"
         },
         "migrations":{
            "airflowDatabase":false
         }
      },
      "4.0.0":{
         "metadata":{
            "airflowVersion":"2.2.0",
            "channel":"stable",
            "releaseDate":"2021-10-12"
         },
         "migrations":{
            "airflowDatabase":true
         }
      },
      "4.0.1":{
         "metadata":{
            "airflowVersion":"2.2.0",
            "channel":"stable",
            "releaseDate":"2021-10-25"
         },
         "migrations":{
            "airflowDatabase":false
         }
      },
      "4.1.0":{
         "metadata":{
            "airflowVersion":"2.2.4",
            "channel":"stable",
            "releaseDate":"2022-02-22"
         },
         "migrations":{
            "airflowDatabase":true
         }
      },
      "4.2.0":{
         "metadata":{
            "airflowVersion":"2.2.4",
            "channel":"stable",
            "releaseDate":"2022-03-08"
         },
         "migrations":{
            "airflowDatabase":false
         }
      },
      "5.0.0":{
         "metadata":{
            "airflowVersion":"2.3.0",
            "channel":"alpha",
            "releaseDate":"2022-04-21"
         },
         "migrations":{
            "airflowDatabase":true
         }
      }
   },
   "schemaVersion":"1.3"
}
`
	// prepare fake response from houston
	api := new(mocks.ClientInterface)
	api.On("GetDeploymentConfig").Return(mockDeploymentConfig, nil)

	output := new(bytes.Buffer)

	clientRuntime := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewBufferString(runtimeResponse)),
			Header:     make(http.Header),
		}
	})
	runtimeHTTPClient := airflowversions.NewClient(clientRuntime, false)

	runtimeTests := []struct {
		runtimeVersion   string
		expectedImageTag string
		expectedError    string
	}{
		{runtimeVersion: "3.0.1", expectedImageTag: "3.0.1", expectedError: ""},
		{runtimeVersion: "", expectedImageTag: "4.2.0", expectedError: ""},
		{runtimeVersion: "9.9.9", expectedImageTag: "4.2.0", expectedError: ""},
	}

	for _, tt := range runtimeTests {
		defaultTag, err := prepareDefaultAirflowImageTag("", tt.runtimeVersion, runtimeHTTPClient, api, output)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, tt.expectedImageTag, defaultTag)
	}
}

func Test_fallbackDefaultAirflowImageTag(t *testing.T) {
	testUtil.InitTestConfig()

	t.Run("astronomer certified", func(t *testing.T) {
		useAstronomerCertified = true
		okResponse := `{
  "version": "1.0",
  "available_releases": []
}`
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
				Header:     make(http.Header),
			}
		})
		httpClient := airflowversions.NewClient(client, true)

		// prepare fake response from houston
		api := new(mocks.ClientInterface)
		api.On("GetDeploymentConfig").Return(mockDeploymentConfig, nil)

		output := new(bytes.Buffer)

		defaultTag, err := prepareDefaultAirflowImageTag("", "", httpClient, api, output)
		assert.NoError(t, err)
		assert.Equal(t, "2.0.0-buster-onbuild", defaultTag)
	})

	t.Run("astro runtime", func(t *testing.T) {
		useAstronomerCertified = false
		okResponse := `{
		"runtimeVersions: {}"
}`

		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
				Header:     make(http.Header),
			}
		})
		httpClient := airflowversions.NewClient(client, false)

		// prepare fake response from houston
		api := new(mocks.ClientInterface)
		api.On("GetDeploymentConfig").Return(mockDeploymentConfig, nil)

		output := new(bytes.Buffer)

		defaultTag, err := prepareDefaultAirflowImageTag("", "", httpClient, api, output)
		assert.NoError(t, err)
		assert.Equal(t, "3.0.0", defaultTag)
	})
}

func Test_prepareDefaultAirflowImageTagHoustonBadRequest(t *testing.T) {
	testUtil.InitTestConfig()
	mockErrorResponse := errors.New(`an error occurred`) //nolint:goerr113
	// prepare fake response from updates.astronomer.io
	okResponse := `{
	  "version": "1.0",
	  "available_releases": [
		{
		  "version": "1.10.5",
		  "level": "new_feature",
		  "url": "https://github.com/astronomer/airflow/releases/tag/1.10.5-11",
		  "release_date": "2020-10-05T20:03:00+00:00",
		  "tags": [
			"1.10.5-alpine3.10-onbuild",
			"1.10.5-buster-onbuild",
			"1.10.5-alpine3.10",
			"1.10.5-buster"
		  ],
		  "channel": "stable"
		}
	  ]
	}`

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	httpClient := airflowversions.NewClient(client, true)

	// prepare fake response from houston
	api := new(mocks.ClientInterface)
	api.On("GetDeploymentConfig").Return(nil, mockErrorResponse)

	output := new(bytes.Buffer)

	defaultTag, err := prepareDefaultAirflowImageTag("2.0.2", "", httpClient, api, output)
	assert.Equal(t, mockErrorResponse.Error(), err.Error())
	assert.Equal(t, "", defaultTag)
}

func Test_prepareDefaultAirflowImageTagHoustonUnauthedRequest(t *testing.T) {
	testUtil.InitTestConfig()

	// prepare fake response from updates.astronomer.io
	okResponse := `{
	  "version": "1.0",
	  "available_releases": [
		{
		  "version": "1.10.5",
		  "level": "new_feature",
		  "url": "https://github.com/astronomer/airflow/releases/tag/1.10.5-11",
		  "release_date": "2020-10-05T20:03:00+00:00",
		  "tags": [
			"1.10.5-alpine3.10-onbuild",
			"1.10.5-buster-onbuild",
			"1.10.5-alpine3.10",
			"1.10.5-buster"
		  ],
		  "channel": "stable"
		}
	  ]
	}`

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	httpClient := airflowversions.NewClient(client, true)

	// prepare fake response from houston
	api := new(mocks.ClientInterface)
	api.On("GetDeploymentConfig").Return(nil, houston.ErrVerboseInaptPermissions)

	output := new(bytes.Buffer)

	defaultTag, err := prepareDefaultAirflowImageTag("2.0.2", "", httpClient, api, output)
	assert.Equal(t, "the --airflow-version flag is not supported if you're not authenticated to Astronomer. Please authenticate and try again", err.Error())
	assert.Equal(t, "", defaultTag)
}
