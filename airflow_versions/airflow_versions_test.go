package airflowversions

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
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

	defaultImageTag, err := GetDefaultImageTag(httpClient, "")
	assert.NoError(t, err)
	assert.Equal(t, "1.10.5-buster-onbuild", defaultImageTag)
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
