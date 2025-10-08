package airflowversions

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	semver "github.com/Masterminds/semver/v3"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestAirflowVersions(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestGetAstronomerCertifiedTag() {
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
			s.NoError(err)
		} else {
			s.EqualError(err, tt.err.Error())
		}
		s.Equal(tt.output, defaultImageTag)
	}
}

func (s *Suite) TestGetAstroRuntimeTag() {
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

	tagToRuntimeVersion3 := map[string]RuntimeVersion{
		"3.0-1": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "3.0.0",
				Channel:        VersionChannelStable,
				ReleaseDate:    "2025-01-01",
			},
			Migrations: RuntimeVersionMigrations{
				AirflowDatabase: false,
			},
		},
		"3.0-2": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "3.0.1",
				Channel:        VersionChannelStable,
				ReleaseDate:    "2025-01-01",
			},
			Migrations: RuntimeVersionMigrations{
				AirflowDatabase: false,
			},
		},
		"3.0-3": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "3.0.1",
				Channel:        VersionChannelStable,
				ReleaseDate:    "2025-01-01",
			},
			Migrations: RuntimeVersionMigrations{
				AirflowDatabase: false,
			},
		},
		"3.0-4": {
			Metadata: RuntimeVersionMetadata{
				AirflowVersion: "3.0.1",
				Channel:        "alpha",
				ReleaseDate:    "2025-01-01",
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
		{airflowVersion: "", output: "3.0-3", err: nil},
		{airflowVersion: "2.1.1", output: "3.0.1", err: nil},
		{airflowVersion: "2.2.2", output: "", err: ErrNoTagAvailable{airflowVersion: "2.2.2"}},
		{airflowVersion: "3.0.0", output: "3.0-1", err: nil},
	}

	for _, tt := range tests {
		defaultImageTag, err := getAstroRuntimeTag(tagToRuntimeVersion, tagToRuntimeVersion3, tt.airflowVersion)
		if tt.err == nil {
			s.NoError(err)
		} else {
			s.EqualError(err, tt.err.Error())
		}
		s.Equal(tt.output, defaultImageTag)
	}
}

func (s *Suite) TestGetDefaultImageTag() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("certified", func() {
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
		httpClient := NewClient(client, true, false)

		defaultImageTag, err := GetDefaultImageTag(httpClient, "", false)
		s.NoError(err)
		s.Equal("2.2.0-onbuild", defaultImageTag)
	})

	s.Run("runtime", func() {
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
		httpClient := NewClient(client, false, false)

		defaultImageTag, err := GetDefaultImageTag(httpClient, "", false)
		s.NoError(err)
		s.Equal("4.0.0", defaultImageTag)
	})

	s.Run("agent", func() {
		mockResp := &Response{
			ClientVersions: map[string]ClientVersion{
				"1.1.0": {
					Metadata: ClientVersionMetadata{
						Channel:     VersionChannelStable,
						ReleaseDate: "2025-09-29",
					},
					ImageTags: []string{
						"3.1-1-python-3.12-astro-agent-1.1.0",
						"3.1-1-python-3.11-astro-agent-1.1.0",
						"3.1-1-python-3.12-astro-agent-1.1.0-base",
						"3.1-1-python-3.11-astro-agent-1.1.0-base",
						"3.0-12-python-3.12-astro-agent-1.1.0",
						"3.0-12-python-3.11-astro-agent-1.1.0",
						"3.0-12-python-3.12-astro-agent-1.1.0-base",
						"3.0-12-python-3.11-astro-agent-1.1.0-base",
						"3.0-8-python-3.12-astro-agent-1.1.0",
						"3.0-8-python-3.11-astro-agent-1.1.0",
						"3.0-8-python-3.12-astro-agent-1.1.0-base",
						"3.0-8-python-3.11-astro-agent-1.1.0-base",
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
		httpClient := NewClient(client, false, true)

		defaultImageTag, err := GetDefaultImageTag(httpClient, "", false)
		s.NoError(err)
		s.Equal("3.1-1-python-3.12-astro-agent-1.1.0", defaultImageTag)
	})
}

func (s *Suite) TestGetDefaultImageTagError() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	okResponse := `Page not found`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	httpClient := NewClient(client, true, false)

	defaultImageTag, err := GetDefaultImageTag(httpClient, "", false)
	s.Error(err)
	s.Equal("", defaultImageTag)
}

func (s *Suite) TestParseImageTag() {
	testCases := []struct {
		name        string
		imageTag    string
		expected    *ImageTagInfo
		shouldError bool
	}{
		{
			name:     "valid non-base image",
			imageTag: "3.1-1-python-3.12-astro-agent-1.1.0",
			expected: &ImageTagInfo{
				RuntimeVersion: "3.1-1",
				PythonVersion:  "3.12",
				AgentVersion:   "1.1.0",
				IsBase:         false,
			},
		},
		{
			name:     "valid base image",
			imageTag: "3.0-12-python-3.11-astro-agent-1.1.0-base",
			expected: &ImageTagInfo{
				RuntimeVersion: "3.0-12",
				PythonVersion:  "3.11",
				AgentVersion:   "1.1.0",
				IsBase:         true,
			},
		},
		{
			name:     "runtime without patch version",
			imageTag: "4.0-python-3.12-astro-agent-2.0.0",
			expected: &ImageTagInfo{
				RuntimeVersion: "4.0",
				PythonVersion:  "3.12",
				AgentVersion:   "2.0.0",
				IsBase:         false,
			},
		},
		{
			name:        "invalid tag format",
			imageTag:    "invalid-tag-format",
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result, err := parseImageTag(tc.imageTag)

			if tc.shouldError {
				s.Error(err)
				s.Nil(result)
			} else {
				s.NoError(err)
				s.NotNil(result)
				s.Equal(tc.expected.RuntimeVersion, result.RuntimeVersion)
				s.Equal(tc.expected.PythonVersion, result.PythonVersion)
				s.Equal(tc.expected.AgentVersion, result.AgentVersion)
				s.Equal(tc.expected.IsBase, result.IsBase)
			}
		})
	}
}

func (s *Suite) TestIsBetterImage() {
	testCases := []struct {
		name      string
		candidate *ImageTagInfo
		current   *ImageTagInfo
		expected  bool
	}{
		{
			name:      "nil candidate",
			candidate: nil,
			current:   &ImageTagInfo{PythonVersion: "3.11"},
			expected:  false,
		},
		{
			name:      "nil current",
			candidate: &ImageTagInfo{PythonVersion: "3.11"},
			current:   nil,
			expected:  true,
		},
		{
			name: "higher python version but lower runtime version",
			candidate: &ImageTagInfo{
				PythonVersion:  "3.12",
				RuntimeVersion: "3.0-8",
			},
			current: &ImageTagInfo{
				PythonVersion:  "3.11",
				RuntimeVersion: "3.1-1",
			},
			expected: false, // Runtime version takes priority, so 3.0-8 < 3.1-1 means false
		},
		{
			name: "lower python version but higher runtime version",
			candidate: &ImageTagInfo{
				PythonVersion:  "3.11",
				RuntimeVersion: "3.1-1",
			},
			current: &ImageTagInfo{
				PythonVersion:  "3.12",
				RuntimeVersion: "3.0-8",
			},
			expected: true, // Runtime version takes priority, so 3.1-1 > 3.0-8 means true
		},
		{
			name: "same python version, higher runtime",
			candidate: &ImageTagInfo{
				PythonVersion:  "3.12",
				RuntimeVersion: "3.1-1",
			},
			current: &ImageTagInfo{
				PythonVersion:  "3.12",
				RuntimeVersion: "3.0-12",
			},
			expected: true,
		},
		{
			name: "same python version, lower runtime",
			candidate: &ImageTagInfo{
				PythonVersion:  "3.12",
				RuntimeVersion: "3.0-12",
			},
			current: &ImageTagInfo{
				PythonVersion:  "3.12",
				RuntimeVersion: "3.1-1",
			},
			expected: false,
		},
		{
			name: "same runtime version, higher python version",
			candidate: &ImageTagInfo{
				PythonVersion:  "3.12",
				RuntimeVersion: "3.1-1",
			},
			current: &ImageTagInfo{
				PythonVersion:  "3.11",
				RuntimeVersion: "3.1-1",
			},
			expected: true, // Same runtime, so Python version is the tiebreaker
		},
		{
			name: "same runtime version, lower python version",
			candidate: &ImageTagInfo{
				PythonVersion:  "3.11",
				RuntimeVersion: "3.1-1",
			},
			current: &ImageTagInfo{
				PythonVersion:  "3.12",
				RuntimeVersion: "3.1-1",
			},
			expected: false, // Same runtime, so Python version is the tiebreaker
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := isBetterImage(tc.candidate, tc.current)
			s.Equal(tc.expected, result)
		})
	}
}
