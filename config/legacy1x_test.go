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

	value, file, _ := Legacy1xValue("webserver.port")
	s.Equal("8099", value, "a port the user chose")
	s.Equal(homeFile, file)
	value, _, _ = Legacy1xValue("api-server.port")
	s.Empty(value, "1.x's default port is no choice")
	value, _, replacement := Legacy1xValue("dev.build_secrets")
	s.Equal("id=mysecret,src=secret.txt", value, "a value the user chose")
	s.Equal(removedSettings["dev.build_secrets"].replacement, replacement, "the advice astro config refuses it with")
	value, _, _ = Legacy1xValue("proxy.port")
	s.Empty(value, "unset")
	s.NotContains(CFGStrMap, "webserver.port", "reading it does not register it")
}

func (s *Suite) TestLegacy1xValueWithNoHomeConfig() {
	s.withAstroHome("empty-home")
	initHome(afero.NewMemMapFs())
	value, _, _ := Legacy1xValue("webserver.port")
	s.Empty(value)
}
