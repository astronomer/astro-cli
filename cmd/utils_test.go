package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

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
	httpClient := airflowversions.NewClient(client)

	// prepare fake response from houston
	ok := `{
  "data": {
    "deploymentConfig": {
      "airflowVersions": [
        "2.1.0",
        "2.0.2",
        "2.0.0",
        "1.10.15",
        "1.10.14",
        "1.10.12",
        "1.10.10",
        "1.10.7",
        "1.10.5"
      ]
    }
  }
}`
	testHoustonClient := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(ok)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewClient(testHoustonClient)

	output := new(bytes.Buffer)

	myTests := []struct {
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
	for _, tt := range myTests {
		defaultTag, err := prepareDefaultAirflowImageTag(tt.airflowVersion, httpClient, api, output)
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

	// prepare fake response from updates.astronomer.io
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
	httpClient := airflowversions.NewClient(client)

	// prepare fake response from houston
	ok := `{
	"data": {
		"deploymentConfig": {
				"airflowVersions": [
					"2.1.0",
					"2.0.2",
					"2.0.0",
					"1.10.15",
					"1.10.14",
					"1.10.12",
					"1.10.10",
					"1.10.7",
					"1.10.5"
				]
			}
		}
	}`
	testHoustonClient := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(ok)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewClient(testHoustonClient)

	output := new(bytes.Buffer)

	myTests := []struct {
		airflowVersion   string
		expectedImageTag string
		expectedError    string
	}{
		{airflowVersion: "", expectedImageTag: "2.0.0-buster-onbuild", expectedError: ""},
	}
	for _, tt := range myTests {
		defaultTag, err := prepareDefaultAirflowImageTag(tt.airflowVersion, httpClient, api, output)
		if tt.expectedError != "" {
			assert.EqualError(t, err, tt.expectedError)
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, tt.expectedImageTag, defaultTag)
	}
}

func Test_prepareDefaultAirflowImageTagHoustonBadRequest(t *testing.T) {
	testUtil.InitTestConfig()
	mockErrorResponse := `An error occurred`
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
	httpClient := airflowversions.NewClient(client)

	// prepare fake response from houston
	testHoustonClient := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 400,
			Body:       ioutil.NopCloser(bytes.NewBufferString(mockErrorResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewClient(testHoustonClient)

	output := new(bytes.Buffer)

	defaultTag, err := prepareDefaultAirflowImageTag("2.0.2", httpClient, api, output)
	assert.Equal(t, err.Error(), fmt.Sprintf("API error (400): %s", mockErrorResponse))
	assert.Equal(t, "", defaultTag)
}

func Test_prepareDefaultAirflowImageTagHoustonUnauthedRequest(t *testing.T) {
	testUtil.InitTestConfig()
	mockErrorResponse := `{
	  "errors": [
		{
		  "message": "You do not have the appropriate permissions for that",
		  "locations": [
			{
			  "line": 2,
			  "column": 3
			}
		  ],
		  "path": [
			"deploymentConfig"
		  ],
		  "extensions": {
			"code": "FORBIDDEN",
			"exception": {
			  "message": "You do not have the appropriate permissions for that"
			}
		  }
		}
	  ],
	  "data": {
		"deploymentConfig": null
	  }
	}`
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
	httpClient := airflowversions.NewClient(client)

	// prepare fake response from houston
	testHoustonClient := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(mockErrorResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewClient(testHoustonClient)

	output := new(bytes.Buffer)

	defaultTag, err := prepareDefaultAirflowImageTag("2.0.2", httpClient, api, output)
	assert.Equal(t, "the --airflow-version flag is not supported if you're not authenticated to Astronomer. Please authenticate and try again", err.Error())
	assert.Equal(t, "", defaultTag)
}
