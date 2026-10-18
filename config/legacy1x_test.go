package config

import (
	"path/filepath"

	"github.com/spf13/afero"
)

// Legacy1xHomeValue reports a 1.x-only setting the user chose, and nothing
// for one that is unset or holds 1.x's shipped default. config_version 2
// keeps the one-time cleanup from removing the defaults before the read, so
// the default case is the value check's, not the migration's.
func (s *Suite) TestLegacy1xHomeValue() {
	fs := afero.NewMemMapFs()
	const home = "home"
	s.withAstroHome(home)
	s.NoError(afero.WriteFile(fs, filepath.Join(home, ConfigDir, ConfigFileNameWithExt), []byte(
		"config_version: 2\n"+
			"webserver:\n  port: 8099\n"+
			"api-server:\n  port: \"8080\"\n"+
			"dev:\n  build_secrets: id=mysecret,src=secret.txt\n"), 0o600))
	initHome(fs)

	s.Equal("8099", Legacy1xHomeValue("webserver.port"), "a port the user chose")
	s.Empty(Legacy1xHomeValue("api-server.port"), "1.x's default port is no choice")
	s.Equal("id=mysecret,src=secret.txt", Legacy1xHomeValue("dev.build_secrets"))
	s.Empty(Legacy1xHomeValue("proxy.port"), "unset")
	s.NotContains(CFGStrMap, "webserver.port", "reading it does not register it")
}

func (s *Suite) TestLegacy1xHomeValueWithNoHomeConfig() {
	s.withAstroHome("empty-home")
	initHome(afero.NewMemMapFs())
	s.Empty(Legacy1xHomeValue("webserver.port"))
}
