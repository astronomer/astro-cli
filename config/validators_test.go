package config

func (s *Suite) Test_validateRegistryEndpoint() {
	s.Run("test valid registry endpoints", func() {
		validEndpoints := []string{
			"quay.io/test/registry",
			"docker.io/user/repo",
			"registry.example.com/namespace/repo",
			"localhost:5000/test/repo",
			"myregistry.example.com:7990/repository/path/my-image", // a port, and a multi-segment path
			"acme/my-image",                                        // Docker Hub, normalized
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
			"myregistry.example.com:7990/repository/path/my-image:1.0", // embedded tag on repository path
		}

		for _, endpoint := range invalidEndpoints {
			err := ValidateRegistryEndpoint(endpoint)
			s.Error(err, "Expected endpoint %s to be invalid", endpoint)
		}
	})

	// What the CLI appends its deploy tag to must be a bare repository, by
	// Docker's own grammar: each is refused with the reason that fits.
	s.Run("refuses what is not a bare repository, saying why", func() {
		for endpoint, want := range map[string]string{
			"myregistry.example.com:7990/repository/path/my-image:1.0":                                          "must not include an image tag",
			"quay.io/acme/my-image:1.0":                                                                         "must not include an image tag",
			"quay.io/acme/my-image@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef":     "must not include an image digest",
			"quay.io/acme/my-image:1.0@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef": "must not include an image tag or digest",
			"quay.io/Acme/My-Image":   "must be lowercase",
			"quay.io/acme/img@latest": "is not a valid image repository",
			"myhost:abc/repo":         "is not a valid image repository",
		} {
			err := ValidateRegistryEndpoint(endpoint)
			s.ErrorContains(err, want, "for %q", endpoint)
		}
	})
}
