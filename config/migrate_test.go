package config

import (
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

// startWithHomeConfig lays down a home config as an older CLI left it and
// starts up against it, in a temp ASTRO_HOME so the lock the cleanup takes
// is not the developer's.
func (s *Suite) startWithHomeConfig(content string) afero.Fs {
	s.withAstroHome(s.T().TempDir())
	fs := afero.NewMemMapFs()
	initHome(fs) // sets HomeConfigFile from ASTRO_HOME; no file, so nothing is written
	s.Require().NoError(afero.WriteFile(fs, HomeConfigFile, []byte(content), 0o600))
	InitConfig(fs)
	return fs
}

func (s *Suite) homeOnDisk(fs afero.Fs) map[string]any {
	data, err := afero.ReadFile(fs, HomeConfigFile)
	s.Require().NoError(err)
	var got map[string]any
	s.Require().NoError(yaml.Unmarshal(data, &got))
	return got
}

// What a v2 build before this one wrote on the first save, cut down: v2's
// defaults, 1.x's, and the user's own context, workspace and page size.
const dumpedHomeConfig = `context: astronomer.io
contexts:
    astronomer_io:
        domain: astronomer.io
        token: Bearer sometoken
        workspace: someworkspace
beta:
    remote_cmd: "false"
cloud:
    api:
        port: "443"
        protocol: https
dev:
    mode: docker
machine:
    cpu: "2"
page_size: "50"
postgres:
    password: postgres
    tag: "12.6"
    user: postgres
show_warnings: "true"
telemetry:
    enabled: "true"
webserver:
    port: "8080"
`

func (s *Suite) TestMigrateHomeConfig() {
	s.Run("drops every default either CLI wrote, and keeps the rest", func() {
		fs := s.startWithHomeConfig(dumpedHomeConfig)

		s.Equal(map[string]any{
			"config_version": currentConfigVersion,
			"context":        "astronomer.io",
			"contexts": map[string]any{"astronomer_io": map[string]any{
				"domain": "astronomer.io", "token": "Bearer sometoken", "workspace": "someworkspace",
			}},
			"page_size": "50",
		}, s.homeOnDisk(fs))
		s.Equal(50, CFG.PageSize.GetInt())
		s.True(CFG.ShowWarnings.GetBool(), "a dropped setting reads as its default")
		s.Equal("default", CFG.ShowWarnings.HomeScope())
	})

	s.Run("keeps a 1.x setting the user chose", func() {
		fs := s.startWithHomeConfig("postgres:\n    tag: \"13.4\"\nwebserver:\n    port: \"8099\"\n")

		got := s.homeOnDisk(fs)
		s.Equal(map[string]any{"tag": "13.4"}, got["postgres"])
		s.Equal(map[string]any{"port": "8099"}, got["webserver"])
	})

	s.Run("a file 1.x already cleaned is cleaned again, for what a v2 build wrote into it since", func() {
		fs := s.startWithHomeConfig("config_version: 1\npage_size: \"20\"\npostgres:\n    tag: \"12.6\"\n")

		s.Equal(map[string]any{"config_version": currentConfigVersion}, s.homeOnDisk(fs))
	})

	s.Run("runs once: a default set after it is the user's", func() {
		fs := s.startWithHomeConfig(dumpedHomeConfig)
		s.Require().NoError(CFG.ShowWarnings.SetHomeString("true"))

		InitConfig(fs)

		s.Equal(true, s.homeOnDisk(fs)["apc"].(map[string]any)["show_warnings"])
		s.Equal("global", CFG.ShowWarnings.HomeScope())
	})

	s.Run("a file this build creates is born stamped", func() {
		s.withAstroHome(s.T().TempDir())
		fs := afero.NewMemMapFs()
		InitConfig(fs)
		s.Require().NoError(CFG.PageSize.SetHomeString("30"))

		s.Equal(map[string]any{"config_version": currentConfigVersion, "apc": map[string]any{"page_size": 30}}, s.homeOnDisk(fs))
	})

	s.Run("an unwritable config is not fatal and is left as it was", func() {
		s.withAstroHome(s.T().TempDir())
		mem := afero.NewMemMapFs()
		initHome(mem)
		s.Require().NoError(afero.WriteFile(mem, HomeConfigFile, []byte(dumpedHomeConfig), 0o600))
		ro := afero.NewReadOnlyFs(mem)

		s.NotPanics(func() { InitConfig(ro) })
		data, err := afero.ReadFile(mem, HomeConfigFile)
		s.Require().NoError(err)
		s.Equal(dumpedHomeConfig, string(data))
		s.Equal(50, CFG.PageSize.GetInt())
	})
}

func (s *Suite) TestDeleteNested() {
	settings := map[string]any{
		"postgres":  map[string]any{"tag": "12.6", "user": "postgres"},
		"page_size": "20",
	}

	deleteNested(settings, "postgres.tag")
	s.Equal(map[string]any{"postgres": map[string]any{"user": "postgres"}, "page_size": "20"}, settings)

	// A parent left empty goes with its last child.
	deleteNested(settings, "postgres.user")
	s.Equal(map[string]any{"page_size": "20"}, settings)

	// Absent keys and non-map parents are left alone.
	deleteNested(settings, "nothing.here")
	deleteNested(settings, "page_size.nested")
	s.Equal(map[string]any{"page_size": "20"}, settings)
}
