package config

import (
	"strings"

	"github.com/spf13/afero"
)

// A setting v2 renamed is still set by its 1.x name, which is what 1.x reads
// and writes in the shared file.
func (s *Suite) TestLegacySettingNames() {
	s.Run("a file setting only the 1.x name sets the setting", func() {
		s.startWithHomeConfig("config_version: 2\npage_size: \"50\"\nhouston:\n    dial_timeout: \"30\"\n")

		s.Equal(50, CFG.PageSize.GetInt())
		s.Equal("30", CFG.HoustonDialTimeout.GetHomeString())
		s.Equal("global", CFG.PageSize.HomeScope())
		s.Equal("default", CFG.ShaAsTag.HomeScope())
	})

	// 1.x reads only the old name, so a value it saved after v2 last wrote
	// the setting is there and nowhere else.
	s.Run("the 1.x name wins while the file holds it", func() {
		s.startWithHomeConfig("config_version: 2\npage_size: \"30\"\napc:\n    page_size: 70\n")

		s.Equal(30, CFG.PageSize.GetInt())
		s.Equal("global", CFG.PageSize.HomeScope())
	})

	s.Run("a set writes both names while the file holds the 1.x one, so 1.x sees it", func() {
		fs := s.startWithHomeConfig("config_version: 2\npage_size: \"50\"\n")

		s.Require().NoError(CFG.PageSize.SetHomeString("70"))

		got := s.homeOnDisk(fs)
		s.Equal(70, got["page_size"])
		s.Equal(map[string]any{"page_size": 70}, got["apc"])
		s.Equal(70, CFG.PageSize.GetInt())
	})

	s.Run("a set adds no 1.x name the file does not hold", func() {
		fs := s.startWithHomeConfig("config_version: 2\n")

		s.Require().NoError(CFG.PageSize.SetHomeString("70"))

		got := s.homeOnDisk(fs)
		s.NotContains(got, "page_size")
		s.Equal(map[string]any{"page_size": 70}, got["apc"])
	})

	// v2 writes, 1.x writes after it, v2 reads: 1.x's value, not v2's older one.
	s.Run("a change 1.x makes after a v2 set is read", func() {
		fs := s.startWithHomeConfig("config_version: 2\npage_size: \"50\"\n")
		s.Require().NoError(CFG.PageSize.SetHomeString("70"))
		data, err := afero.ReadFile(fs, HomeConfigFile)
		s.Require().NoError(err)
		// What 1.x's own set would do: change the top-level page_size, carry
		// the rest, apc.page_size included.
		s.Require().Contains(string(data), "\npage_size: 70\n")
		s.Require().NoError(afero.WriteFile(fs, HomeConfigFile, []byte(strings.Replace(string(data), "\npage_size: 70\n", "\npage_size: 30\n", 1)), 0o600))

		InitConfig(fs)

		s.Equal(30, CFG.PageSize.GetInt())
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
		_, removed := removedSettings[legacy]
		s.False(removed, "%s is both a legacy name and a removed setting", legacy)
		reason, refused := RemovedSetting(legacy)
		s.True(refused, "astro config takes %s, a 1.x name", legacy)
		s.Equal("use "+current, reason)
	}
	for name := range removedSettings {
		_, clash := CFGStrMap[name]
		s.False(clash, "%s is both removed and registered", name)
	}
}
