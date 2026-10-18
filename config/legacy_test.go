package config

import (
	"github.com/spf13/afero"
)

// A setting v2 renamed is still set by its 1.x name, which is what 1.x reads
// and writes in the shared file.
func (s *Suite) TestLegacySettingNames() {
	s.Run("a file setting only the 1.x name sets the setting", func() {
		s.startWithHomeConfig("config_version: 2\nshow_warnings: false\npage_size: \"50\"\nhouston:\n    dial_timeout: \"30\"\n")

		s.False(CFG.ShowWarnings.GetBool())
		s.Equal(50, CFG.PageSize.GetInt())
		s.Equal("30", CFG.HoustonDialTimeout.GetHomeString())
		s.Equal("global", CFG.PageSize.HomeScope())
		s.Equal("default", CFG.ShaAsTag.HomeScope())
	})

	s.Run("the current name wins over the 1.x one", func() {
		s.startWithHomeConfig("config_version: 2\npage_size: \"50\"\napc:\n    page_size: 70\n")

		s.Equal(70, CFG.PageSize.GetInt())
	})

	s.Run("a set writes the current name and leaves the 1.x one for 1.x", func() {
		fs := s.startWithHomeConfig("config_version: 2\npage_size: \"50\"\n")

		s.Require().NoError(CFG.PageSize.SetHomeString("70"))

		got := s.homeOnDisk(fs)
		s.Equal("50", got["page_size"])
		s.Equal(map[string]any{"page_size": 70}, got["apc"])
		s.Equal(70, CFG.PageSize.GetInt())
	})

	s.Run("a 1.x project config read by its path takes the 1.x name", func() {
		s.startWithHomeConfig("config_version: 2\n")
		dir := s.T().TempDir()
		s.Require().NoError(afero.WriteFile(configFs, dir+"/.astro/config.yaml", []byte("remote:\n    client_registry: quay.io/acme/client\n"), 0o600))

		s.Equal("quay.io/acme/client", CFG.RemoteClientRegistry.GetStringFor(dir))
	})

	s.Run("the cleanup removes a default written under the 1.x name", func() {
		fs := s.startWithHomeConfig("show_warnings: \"true\"\ncloud:\n    api:\n        port: \"443\"\nauto_select: \"true\"\n")

		s.Equal(map[string]any{"config_version": currentConfigVersion, "auto_select": "true"}, s.homeOnDisk(fs))
		s.True(CFG.AutoSelect.GetBool())
	})
}

// Every legacy name belongs to a registered setting and is no name in use
// otherwise: not a current one, and not a removed one, which get and set
// refuse rather than map.
func (s *Suite) TestLegacySettingNamesAreUnambiguous() {
	for legacy, current := range LegacySettingNames {
		_, registered := CFGStrMap[current]
		s.True(registered, "%s maps to %s, which is not a setting", legacy, current)
		_, clash := CFGStrMap[legacy]
		s.False(clash, "%s is both a legacy name and a current one", legacy)
		_, removed := RemovedSetting(legacy)
		s.False(removed, "%s is both a legacy name and a removed setting", legacy)
	}
	for name := range removedSettings {
		_, clash := CFGStrMap[name]
		s.False(clash, "%s is both removed and registered", name)
	}
}
