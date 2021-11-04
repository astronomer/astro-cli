package airflowversions

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"

	"github.com/Masterminds/semver"
)

func TestGetDefaultImageTag(t *testing.T) {
	testUtil.InitTestConfig()
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
    },
    {
      "version": "1.10.5-11",
      "level": "bug_fix",
      "url": "https://github.com/astronomer/airflow/releases/tag/1.10.5-11",
      "release_date": "2020-10-05T20:03:00+00:00",
      "tags": [
        "1.10.5-11-alpine3.10-onbuild",
        "1.10.5-11-buster-onbuild",
        "1.10.5-11-alpine3.10",
        "1.10.5-11-buster"
      ],
      "channel": "stable"
    },
    {
      "version": "1.10.4-11",
      "level": "bug_fix",
      "url": "https://github.com/astronomer/airflow/releases/tag/1.10.4-11",
      "release_date": "2020-9-05T20:03:00+00:00",
      "tags": [
        "1.10.4-11-alpine3.10-onbuild",
        "1.10.4-11-buster-onbuild",
        "1.10.4-11-alpine3.10",
        "1.10.4-11-buster"
      ],
      "channel": "stable"
    },
    {
      "version": "2.2.0",
      "level": "new_feature",
      "url": "https://github.com/astronomer/airflow/releases/tag/v2.2.0%2Bastro.2",
      "release_date": "2021-10-14T12:46:00+00:00",
      "tags": ["2.2.0", "2.2.0-onbuild"],
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
	httpClient := NewClient(client)

	tests := []struct {
		airflowVersion string
		output         string
		err            error
	}{
		{airflowVersion: "", output: "2.2.0-onbuild", err: nil},
		{airflowVersion: "1.10.5", output: "1.10.5-buster-onbuild", err: nil},
		{airflowVersion: "1.10.4-rc.1", output: "1.10.4-rc.1", err: nil},
		{airflowVersion: "2.2.1", output: "2.2.1", err: nil},
		{airflowVersion: "2.2.0", output: "2.2.0-onbuild", err: nil},
		{airflowVersion: "2.2.x", output: "", err: semver.ErrInvalidSemVer},
	}

	for _, tt := range tests {
		defaultImageTag, err := GetDefaultImageTag(httpClient, tt.airflowVersion)
		if tt.err == nil {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, tt.err.Error())
		}
		assert.Equal(t, tt.output, defaultImageTag)
	}
}

func TestGetDefaultImageTagError(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `Page not found`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 404,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	httpClient := NewClient(client)

	defaultImageTag, err := GetDefaultImageTag(httpClient, "")
	assert.Error(t, err)
	assert.Equal(t, "", defaultImageTag)
}
