package config

import (
	"path/filepath"

	"github.com/spf13/afero"
)

// Legacy1xValue reports a 1.x-only setting the user chose, and nothing for
// one that is unset or holds 1.x's shipped default. config_version 2 keeps
// the one-time cleanup from removing the defaults before the read, so the
// default case is the value check's, not the migration's.
func (s *Suite) TestLegacy1xValueFromTheHomeConfig() {
	fs := afero.NewMemMapFs()
	const home = "home"
	s.withAstroHome(home)
	homeFile := filepath.Join(home, ConfigDir, ConfigFileNameWithExt)
	s.NoError(afero.WriteFile(fs, homeFile, []byte(
		"config_version: 2\n"+
			"webserver:\n  port: 8099\n"+
			"api-server:\n  port: \"8080\"\n"+
			"dev:\n  build_secrets: id=mysecret,src=secret.txt\n"), 0o600))
	initHome(fs)

	value, file := Legacy1xValue("", "webserver.port")
	s.Equal("8099", value, "a port the user chose")
	s.Equal(homeFile, file)
	value, _ = Legacy1xValue("", "api-server.port")
	s.Empty(value, "1.x's default port is no choice")
	value, _ = Legacy1xValue("no-config-here", "dev.build_secrets")
	s.Equal("id=mysecret,src=secret.txt", value, "a project with no .astro/config.yaml reads the home config")
	value, _ = Legacy1xValue("", "proxy.port")
	s.Empty(value, "unset")
	s.NotContains(CFGStrMap, "webserver.port", "reading it does not register it")
}

// 1.x's `astro config set` without -g wrote the project's .astro/config.yaml,
// which it read over the home config, at any value.
func (s *Suite) TestLegacy1xValueFromTheProjectConfig() {
	fs := afero.NewMemMapFs()
	const home = "home"
	s.withAstroHome(home)
	s.NoError(afero.WriteFile(fs, filepath.Join(home, ConfigDir, ConfigFileNameWithExt), []byte(
		"config_version: 2\nwebserver:\n  port: 8099\napi-server:\n  port: 9090\n"), 0o600))
	projectFile := filepath.Join("proj", ConfigDir, ConfigFileNameWithExt)
	s.NoError(afero.WriteFile(fs, projectFile, []byte(
		"webserver:\n  port: 8100\napi-server:\n  port: 8080\n"), 0o600))
	initHome(fs)

	value, file := Legacy1xValue("proj", "webserver.port")
	s.Equal("8100", value)
	s.Equal(projectFile, file, "the note names the file the value came from")
	value, _ = Legacy1xValue("proj", "api-server.port")
	s.Empty(value, "the project's default overrides the home value, as in 1.x")
}

func (s *Suite) TestLegacy1xValueWithNoHomeConfig() {
	s.withAstroHome("empty-home")
	initHome(afero.NewMemMapFs())
	value, _ := Legacy1xValue("", "webserver.port")
	s.Empty(value)
}
