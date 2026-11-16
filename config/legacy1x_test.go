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

// What astro init says about each setting a converted 1.x project's file held:
// its v2 name to set globally, what replaces a removed one, or nothing.
func (s *Suite) TestProjectSetting1x() {
	s.restoreConfigGlobals()
	initTestConfig()
	global, removed, ok := ProjectSetting1x("remote.client_registry", "quay.io/acme/client")
	s.True(ok)
	s.Equal("deploy.remote_execution.client_registry", global, "a 1.x name is set under its v2 one")
	s.Empty(removed)

	global, _, ok = ProjectSetting1x("container.binary", "podman")
	s.True(ok)
	s.Equal("container.binary", global)

	global, removed, ok = ProjectSetting1x("webserver.port", "8099")
	s.True(ok)
	s.Empty(global)
	s.Equal(removedSettings["webserver.port"].replacement, removed)

	_, _, ok = ProjectSetting1x("webserver.port", "8080")
	s.False(ok, "a removed setting at the value it shipped with")
	_, _, ok = ProjectSetting1x("show_warnings", CFG.ShowWarnings.Default)
	s.False(ok, "a setting at its default")
	_, _, ok = ProjectSetting1x("not.a.setting", "x")
	s.False(ok)
	_, _, ok = ProjectSetting1x(contextsKey, "x")
	s.False(ok, "logins are never a project's")

	// 1.x read the project's value over the global one, so the default set in
	// a project over a global value that differs is a change worth naming.
	s.Require().NoError(CFG.UpgradeMessage.SetHomeString("false"))
	global, _, ok = ProjectSetting1x("upgrade_message", CFG.UpgradeMessage.Default)
	s.True(ok, "the project's default shadowed the global false")
	s.Equal("upgrade_message", global)
	_, _, ok = ProjectSetting1x("upgrade_message", "false")
	s.False(ok, "the value this machine already uses")

	global, _, ok = ProjectSetting1x("Container.Binary", "podman")
	s.True(ok, "keys match in any case, as viper read them")
	s.Equal("container.binary", global)
}
