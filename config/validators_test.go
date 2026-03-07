package config

func (s *Suite) Test_validateRegistryEndpoint() {
	s.Run("test valid registry endpoints", func() {
		validEndpoints := []string{
			"quay.io/test/registry",
			"docker.io/user/repo",
			"registry.example.com/namespace/repo",
			"localhost:5000/test/repo",
		}

		for _, endpoint := range validEndpoints {
			err := ValidateRegistryEndpoint(endpoint)
			s.NoError(err, "Expected endpoint %s to be valid", endpoint)
		}
	})

	s.Run("test invalid registry endpoints", func() {
		invalidEndpoints := []string{
			"",                          // empty
			"invalid-registry",          // no slash
			"registry with spaces/repo", // contains spaces
			"/registry/repo",            // starts with slash
			"registry/repo/",            // ends with slash
		}

		for _, endpoint := range invalidEndpoints {
			err := ValidateRegistryEndpoint(endpoint)
			s.Error(err, "Expected endpoint %s to be invalid", endpoint)
		}
	})
}

func (s *Suite) Test_validateDevMode() {
	s.Run("accepts docker", func() {
		err := ValidateDevMode("docker")
		s.NoError(err)
	})

	s.Run("accepts standalone", func() {
		err := ValidateDevMode("standalone")
		s.NoError(err)
	})

	s.Run("rejects invalid values", func() {
		invalidValues := []string{"", "invalid", "Docker", "STANDALONE", "local", "compose"}
		for _, v := range invalidValues {
			err := ValidateDevMode(v)
			s.Error(err, "Expected value %q to be invalid", v)
			s.Contains(err.Error(), "dev.mode must be")
		}
	})
}
