package airflowversions

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"

	semver "github.com/Masterminds/semver/v3"
)

func TestGetAstronomerCertifiedTag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	availableReleases := []AirflowVersionRaw{
		{
			Version:     "1.10.5",
			Level:       "new_feature",
			ReleaseDate: "2020-10-05T20:03:00+00:00",
			Tags: []string{
				"1.10.5-alpine3.10-onbuild",
				"1.10.5-buster-onbuild",
				"1.10.5-alpine3.10",
				"1.10.5-buster",
			},
			Channel: VersionChannelStable,
		},
		{
			Version:     "1.10.5-11",
			Level:       "bug_fix",
			ReleaseDate: "2020-10-05T20:03:00+00:00",
			Tags: []string{
				"1.10.5-11-alpine3.10-onbuild",
				"1.10.5-11-buster-onbuild",
				"1.10.5-11-alpine3.10",
				"1.10.5-11-buster",
			},
			Channel: VersionChannelStable,
		},
		{
			Version:     "1.10.4-11",
			Level:       "bug_fix",
			ReleaseDate: "2020-9-05T20:03:00+00:00",
			Tags: []string{
				"1.10.4-11-alpine3.10-onbuild",
				"1.10.4-11-buster-onbuild",
				"1.10.4-11-alpine3.10",
				"1.10.4-11-buster",
			},
			Channel: VersionChannelStable,
		},
		{
			Version:     "2.2.0",
			Level:       "new_feature",
			ReleaseDate: "2021-10-14T12:46:00+00:00",
			Tags: []string{
				"2.2.0",
				"2.2.0-onbuild",
			},
			Channel: VersionChannelStable,
		},
	}

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
		defaultImageTag, err := getAstronomerCertifiedTag(availableReleases, tt.airflowVersion)
		if tt.err == nil {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, tt.err.Error())
		}
		assert.Equal(t, tt.output, defaultImageTag)
	}
}

func TestGetAstroRuntimeTag(t *testing.T) {
	tagToRuntimeVersion := map[string]RuntimeVersion{
		"2.1.1": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "2.1.1",
				Channel:        "deprecated",
				ReleaseDate:    "2021-07-20",
			},
			Migrations: RuntimeVersionMigrations{
				AirflowDatabase: true,
			},
		},
		"3.0.0": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "2.1.1",
				Channel:        VersionChannelStable,
				ReleaseDate:    "2021-08-12",
			},
			Migrations: RuntimeVersionMigrations{
				AirflowDatabase: false,
			},
		},
		"3.0.1": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "2.1.1",
				Channel:        VersionChannelStable,
				ReleaseDate:    "2021-08-31",
			},
			Migrations: RuntimeVersionMigrations{
				AirflowDatabase: false,
			},
		},
		"3.1.0": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "2.1.1",
				Channel:        "alpha",
				ReleaseDate:    "2021-08-31",
			},
			Migrations: RuntimeVersionMigrations{
				AirflowDatabase: false,
			},
		},
		"4.0.0": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "2.2.0",
				Channel:        VersionChannelStable,
				ReleaseDate:    "2021-10-12",
			},
			Migrations: RuntimeVersionMigrations{
				AirflowDatabase: false,
			},
		},
	}

	tests := []struct {
		airflowVersion string
		output         string
		err            error
	}{
		{airflowVersion: "", output: "4.0.0", err: nil},
		{airflowVersion: "2.1.1", output: "3.0.1", err: nil},
		{airflowVersion: "2.2.2", output: "", err: ErrNoTagAvailable{airflowVersion: "2.2.2"}},
	}

	for _, tt := range tests {
		defaultImageTag, err := getAstroRuntimeTag(tagToRuntimeVersion, tt.airflowVersion)
		if tt.err == nil {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, tt.err.Error())
		}
		assert.Equal(t, tt.output, defaultImageTag)
	}
}

func TestGetDefaultImageTag(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("certified", func(t *testing.T) {
		mockResp := &Response{
			AvailableReleases: []AirflowVersionRaw{
				{
					Version:     "1.10.5",
					Level:       "new_feature",
					ReleaseDate: "2020-10-05T20:03:00+00:00",
					Tags: []string{
						"1.10.5-alpine3.10-onbuild",
						"1.10.5-buster-onbuild",
						"1.10.5-alpine3.10",
						"1.10.5-buster",
					},
					Channel: VersionChannelStable,
				},
				{
					Version:     "1.10.5-11",
					Level:       "bug_fix",
					ReleaseDate: "2020-10-05T20:03:00+00:00",
					Tags: []string{
						"1.10.5-11-alpine3.10-onbuild",
						"1.10.5-11-buster-onbuild",
						"1.10.5-11-alpine3.10",
						"1.10.5-11-buster",
					},
					Channel: VersionChannelStable,
				},
				{
					Version:     "1.10.4-11",
					Level:       "bug_fix",
					ReleaseDate: "2020-9-05T20:03:00+00:00",
					Tags: []string{
						"1.10.4-11-alpine3.10-onbuild",
						"1.10.4-11-buster-onbuild",
						"1.10.4-11-alpine3.10",
						"1.10.4-11-buster",
					},
					Channel: VersionChannelStable,
				},
				{
					Version:     "2.2.0",
					Level:       "new_feature",
					ReleaseDate: "2021-10-14T12:46:00+00:00",
					Tags: []string{
						"2.2.0",
						"2.2.0-onbuild",
					},
					Channel: VersionChannelStable,
				},
			},
		}
		jsonResp, _ := json.Marshal(mockResp)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResp)),
				Header:     make(http.Header),
			}
		})
		httpClient := NewClient(client, true)

		defaultImageTag, err := GetDefaultImageTag(httpClient, "")
		assert.NoError(t, err)
		assert.Equal(t, "2.2.0-onbuild", defaultImageTag)
	})

	t.Run("runtime", func(t *testing.T) {
		mockResp := &Response{
			RuntimeVersions: map[string]RuntimeVersion{
				"2.1.1": {
					Metadata: RuntimeVersionMetadata{
						AirflowVersion: "2.1.1",
						Channel:        "deprecated",
						ReleaseDate:    "2021-07-20",
					},
					Migrations: RuntimeVersionMigrations{
						AirflowDatabase: true,
					},
				},
				"3.0.0": {
					Metadata: RuntimeVersionMetadata{
						AirflowVersion: "2.1.1",
						Channel:        VersionChannelStable,
						ReleaseDate:    "2021-08-12",
					},
					Migrations: RuntimeVersionMigrations{
						AirflowDatabase: false,
					},
				},
				"3.0.1": {
					Metadata: RuntimeVersionMetadata{
						AirflowVersion: "2.1.1",
						Channel:        VersionChannelStable,
						ReleaseDate:    "2021-08-31",
					},
					Migrations: RuntimeVersionMigrations{
						AirflowDatabase: false,
					},
				},
				"3.1.0": {
					Metadata: RuntimeVersionMetadata{
						AirflowVersion: "2.1.1",
						Channel:        "alpha",
						ReleaseDate:    "2021-08-31",
					},
					Migrations: RuntimeVersionMigrations{
						AirflowDatabase: false,
					},
				},
				"4.0.0": {
					Metadata: RuntimeVersionMetadata{
						AirflowVersion: "2.2.0",
						Channel:        VersionChannelStable,
						ReleaseDate:    "2021-10-12",
					},
					Migrations: RuntimeVersionMigrations{
						AirflowDatabase: false,
					},
				},
			},
		}
		jsonResp, _ := json.Marshal(mockResp)
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResp)),
				Header:     make(http.Header),
			}
		})
		httpClient := NewClient(client, false)

		defaultImageTag, err := GetDefaultImageTag(httpClient, "")
		assert.NoError(t, err)
		assert.Equal(t, "4.0.0", defaultImageTag)
	})
}

func TestGetDefaultImageTagError(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	okResponse := `Page not found`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	httpClient := NewClient(client, true)

	defaultImageTag, err := GetDefaultImageTag(httpClient, "")
	assert.Error(t, err)
	assert.Equal(t, "", defaultImageTag)
}
