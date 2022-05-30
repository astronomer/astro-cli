package houston

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetValidTagsSimpleSemVer(t *testing.T) {
	dCfg := DeploymentConfig{
		AirflowImages: []AirflowImage{
			{Tag: "1.10.5-11-alpine3.10-onbuild", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-buster-onbuild", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-alpine3.10", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-buster", Version: "1.10.5-11"},
			{Tag: "1.10.5-buster-onbuild", Version: "1.10.5"},
			{Tag: "1.10.5-alpine3.10", Version: "1.10.5"},
			{Tag: "1.10.5-buster", Version: "1.10.5"},
			{Tag: "1.10.5-alpine3.10-onbuild", Version: "1.10.5"},
			{Tag: "1.10.7-7-buster-onbuild", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-alpine3.10", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-buster", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-alpine3.10-onbuild", Version: "1.10.7-7"},
			{Tag: "1.10.7-8-alpine3.10-onbuild", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-buster-onbuild", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-alpine3.10", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-buster", Version: "1.10.7-8"},
		},
	}

	t.Run("tag uses semver", func(t *testing.T) {
		validTags := dCfg.GetValidTags("1.10.7")
		expectedTags := []string{
			"1.10.7-7-buster-onbuild",
			"1.10.7-7-alpine3.10",
			"1.10.7-7-buster",
			"1.10.7-7-alpine3.10-onbuild",
			"1.10.7-8-alpine3.10-onbuild",
			"1.10.7-8-buster-onbuild",
			"1.10.7-8-alpine3.10",
			"1.10.7-8-buster",
		}
		assert.Equal(t, expectedTags, validTags)
	})

	t.Run("tag does not use semver", func(t *testing.T) {
		validTags := dCfg.GetValidTags("buster-1.10.2")
		assert.Equal(t, 0, len(validTags))
	})

	t.Run("get valid tag with pre-release tag", func(t *testing.T) {
		dCfg = DeploymentConfig{
			AirflowImages: []AirflowImage{
				{Tag: "1.10.5-11-alpine3.10-onbuild", Version: "1.10.5-11"},
				{Tag: "1.10.5-11-buster-onbuild", Version: "1.10.5-11"},
				{Tag: "1.10.5-11-alpine3.10", Version: "1.10.5-11"},
				{Tag: "1.10.5-11-buster", Version: "1.10.5-11"},
				{Tag: "1.10.5-buster-onbuild", Version: "1.10.5"},
				{Tag: "1.10.5-alpine3.10", Version: "1.10.5"},
				{Tag: "1.10.5-buster", Version: "1.10.5"},
				{Tag: "1.10.5-alpine3.10-onbuild", Version: "1.10.5"},
				{Tag: "1.10.7-7-buster-onbuild", Version: "1.10.7-7"},
				{Tag: "1.10.7-7-alpine3.10", Version: "1.10.7-7"},
				{Tag: "1.10.7-7-buster", Version: "1.10.7-7"},
				{Tag: "1.10.7-7-alpine3.10-onbuild", Version: "1.10.7-7"},
				{Tag: "1.10.7-8-alpine3.10-onbuild", Version: "1.10.7-8"},
				{Tag: "1.10.7-8-buster-onbuild", Version: "1.10.7-8"},
				{Tag: "1.10.7-8-alpine3.10", Version: "1.10.7-8"},
				{Tag: "1.10.7-8-buster", Version: "1.10.7-8"},
				{Tag: "1.10.12-buster", Version: "1.10.12"},
				{Tag: "1.10.12-buster-onbuild", Version: "1.10.12"},
				{Tag: "1.10.12-1-buster-onbuild", Version: "1.10.12-1"},
			},
		}

		validTags := dCfg.GetValidTags("1.10.12-1-alpine3.10")
		expectedTags := []string{"1.10.12-buster", "1.10.12-buster-onbuild", "1.10.12-1-buster-onbuild"}
		assert.Equal(t, expectedTags, validTags)
	})
}

func TestIsValidTag(t *testing.T) {
	dCfg := DeploymentConfig{
		AirflowImages: []AirflowImage{
			{Tag: "1.10.5-11-alpine3.10-onbuild", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-buster-onbuild", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-alpine3.10", Version: "1.10.5-11"},
			{Tag: "1.10.5-11-buster", Version: "1.10.5-11"},
			{Tag: "1.10.5-buster-onbuild", Version: "1.10.5"},
			{Tag: "1.10.5-alpine3.10", Version: "1.10.5"},
			{Tag: "1.10.5-buster", Version: "1.10.5"},
			{Tag: "1.10.5-alpine3.10-onbuild", Version: "1.10.5"},
			{Tag: "1.10.7-7-buster-onbuild", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-alpine3.10", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-buster", Version: "1.10.7-7"},
			{Tag: "1.10.7-7-alpine3.10-onbuild", Version: "1.10.7-7"},
			{Tag: "1.10.7-8-alpine3.10-onbuild", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-buster-onbuild", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-alpine3.10", Version: "1.10.7-8"},
			{Tag: "1.10.7-8-buster", Version: "1.10.7-8"},
		},
	}

	t.Run("success", func(t *testing.T) {
		resp := dCfg.IsValidTag("1.10.5-11-buster-onbuild")
		assert.True(t, resp)
	})

	t.Run("failure", func(t *testing.T) {
		resp := dCfg.IsValidTag("2.2.5-onbuild")
		assert.False(t, resp)
	})
}

func Test_coerce(t *testing.T) {
	tests := []struct {
		tag             string
		expectedVersion string
		expectError     bool
	}{
		{tag: "1.10.5-11-alpine3.10-onbuild", expectedVersion: "1.10.5", expectError: false},
		{tag: "1.10.5", expectedVersion: "1.10.5", expectError: false},
		{tag: "1.10.12-buster", expectedVersion: "1.10.12", expectError: false},
		{tag: "buster", expectedVersion: "", expectError: true},
	}

	for _, tt := range tests {
		test, err := coerce(tt.tag)
		if tt.expectError {
			assert.Error(t, err)
		} else {
			assert.Equal(t, tt.expectedVersion, test.String())
			assert.NoError(t, err)
		}
	}
}

func TestRuntimeReleasesIsValidVersion(t *testing.T) {
	r := RuntimeReleases{
		RuntimeRelease{Version: "5.0.1", AirflowVersion: "2.2.5", AirflowDBMigraion: false},
		RuntimeRelease{Version: "5.0.2", AirflowVersion: "2.2.6", AirflowDBMigraion: false},
	}
	t.Run("valid", func(t *testing.T) {
		out := r.IsValidVersion("5.0.1")
		assert.True(t, out)
	})

	t.Run("invalid", func(t *testing.T) {
		out := r.IsValidVersion("5.0.0")
		assert.False(t, out)
	})
}

func TestRuntimeReleasesGreaterVersions(t *testing.T) {
	r := RuntimeReleases{
		RuntimeRelease{Version: "5.0.1", AirflowVersion: "2.2.5", AirflowDBMigraion: false},
		RuntimeRelease{Version: "5.0.2", AirflowVersion: "2.2.6", AirflowDBMigraion: false},
	}

	t.Run("valid version", func(t *testing.T) {
		out := r.GreaterVersions("5.0.0")
		assert.Equal(t, []string{"5.0.1", "5.0.2"}, out)
	})

	t.Run("invalid version", func(t *testing.T) {
		out := r.GreaterVersions("invalid version")
		assert.Empty(t, out)
	})
}
