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

// ClientRegistry checks a saved value as a new one is checked, and its error
// names the key that holds it and the command that fixes it, with -g: the
// global config is the only one a setting is read from.
func (s *Suite) TestClientRegistry() {
	s.restoreConfigGlobals()
	initTestConfig()

	s.Run("unset", func() {
		s.Require().NoError(CFG.RemoteClientRegistry.SetHomeString(""))
		got, err := ClientRegistry()
		s.NoError(err)
		s.Empty(got)
	})

	s.Run("a valid saved value", func() {
		s.Require().NoError(CFG.RemoteClientRegistry.SetHomeString("quay.io/acme/my-image"))
		got, err := ClientRegistry()
		s.NoError(err)
		s.Equal("quay.io/acme/my-image", got)
	})

	s.Run("a tagged global value says to fix it with -g", func() {
		s.Require().NoError(CFG.RemoteClientRegistry.SetHomeString("quay.io/acme/my-image:1.0"))
		_, err := ClientRegistry()
		s.ErrorContains(err, "deploy.remote_execution.client_registry in your global config is not a usable registry endpoint")
		s.ErrorContains(err, "must not include an image tag")
		s.ErrorContains(err, "astro config set -g deploy.remote_execution.client_registry")
	})

	// A value 1.x saved under its own name is read, reported under that
	// name, as the user finds it in the file, and fixed under the current
	// one, which wins over it.
	s.Run("a tagged value under the 1.x name", func() {
		initTestConfig()
		viperHome.Set("remote.client_registry", "quay.io/acme/my-image:1.0")
		_, err := ClientRegistry()
		s.ErrorContains(err, "remote.client_registry in your global config is not a usable registry endpoint")
		s.NotContains(err.Error(), "deploy.remote_execution.client_registry in")
		s.ErrorContains(err, "astro config set -g deploy.remote_execution.client_registry")
	})

	// An unset registry is set where it is read: globally, with -g.
	s.Run("the fix for an unset value", func() {
		s.Require().NoError(CFG.RemoteClientRegistry.SetHomeString(""))
		s.Equal("astro config set -g deploy.remote_execution.client_registry <registry>/<repository>", ClientRegistryFix())
	})
}
