package houston

func (s *Suite) TestGetValidTagsSimpleSemVer() {
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

	s.Run("tag uses semver", func() {
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
		s.Equal(expectedTags, validTags)
	})

	s.Run("tag does not use semver", func() {
		validTags := dCfg.GetValidTags("buster-1.10.2")
		s.Equal(0, len(validTags))
	})

	s.Run("get valid tag with pre-release tag", func() {
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
		s.Equal(expectedTags, validTags)
	})
}

func (s *Suite) TestIsValidTag() {
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

	s.Run("success", func() {
		resp := dCfg.IsValidTag("1.10.5-11-buster-onbuild")
		s.True(resp)
	})

	s.Run("failure", func() {
		resp := dCfg.IsValidTag("2.2.5-onbuild")
		s.False(resp)
	})
}

func (s *Suite) Test_coerce() {
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
			s.Error(err)
		} else {
			s.Equal(tt.expectedVersion, test.String())
			s.NoError(err)
		}
	}
}

func (s *Suite) TestRuntimeReleasesIsValidVersion() {
	r := RuntimeReleases{
		RuntimeRelease{Version: "5.0.1", AirflowVersion: "2.2.5", AirflowDBMigraion: false},
		RuntimeRelease{Version: "5.0.2", AirflowVersion: "2.2.6", AirflowDBMigraion: false},
	}
	s.Run("valid", func() {
		out := r.IsValidVersion("5.0.1")
		s.True(out)
	})

	s.Run("invalid", func() {
		out := r.IsValidVersion("5.0.0")
		s.False(out)
	})
}

func (s *Suite) TestRuntimeReleasesGreaterVersions() {
	r := RuntimeReleases{
		RuntimeRelease{Version: "5.0.1", AirflowVersion: "2.2.5", AirflowDBMigraion: false},
		RuntimeRelease{Version: "5.0.2", AirflowVersion: "2.2.6", AirflowDBMigraion: false},
	}

	s.Run("valid version", func() {
		out := r.GreaterVersions("5.0.0")
		s.Equal([]string{"5.0.1", "5.0.2"}, out)
	})

	s.Run("invalid version", func() {
		out := r.GreaterVersions("invalid version")
		s.Empty(out)
	})
}
