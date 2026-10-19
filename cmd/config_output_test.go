package cmd

import (
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
)

// runConfigCmd runs `astro config` with args the way main runs it.
func runConfigCmd(args ...string) (stdout, stderr string, err error) {
	return runCommands(func(out io.Writer) []*cobra.Command {
		return []*cobra.Command{newConfigRootCmd(out)}
	}, append([]string{"config"}, args...)...)
}

// with1xProjectOnDisk writes the 1.x project config initConfigFiles loaded to
// disk as well, since the pre-run looks for the project there.
func (s *CmdSuite) with1xProjectOnDisk(project string) {
	dir := filepath.Join(config.WorkingPath, config.ConfigDir)
	s.Require().NoError(os.MkdirAll(dir, 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(dir, config.ConfigFileNameWithExt), []byte(project), 0o600))
}

func (s *CmdSuite) TestConfigGetOutput() {
	s.Run("text is the line it always printed", func() {
		s.initConfigFiles("page_size: 50\n", "")
		stdout, _, err := runConfigCmd("get", "apc.page_size", "-g")
		s.NoError(err)
		s.Equal("apc.page_size: 50\n", stdout)
	})

	s.Run("json with -g publishes the global value, and whether the home config sets it", func() {
		s.initConfigFiles("page_size: 50\n", "")
		stdout, stderr, err := runConfigCmd("get", "apc.page_size", "-g", "-o", "json")
		s.NoError(err)
		s.Empty(stderr)
		s.Equal(configSetting{Key: "apc.page_size", Value: "50", Scope: "global", Set: true}, decodeOne[configSetting](s, stdout))

		stdout, _, err = runConfigCmd("get", "apc.show_warnings", "-g", "-o", "json")
		s.NoError(err)
		s.Equal(configSetting{Key: "apc.show_warnings", Value: "true", Scope: "global", Set: false}, decodeOne[configSetting](s, stdout),
			"the built-in default, as the text prints it")
	})

	s.Run("json in a 1.x project publishes the project's own value", func() {
		s.initConfigFiles("page_size: 50\n", "page_size: 70\n")
		s.with1xProjectOnDisk("page_size: 70\n")
		stdout, _, err := runConfigCmd("get", "apc.page_size", "-o", "json")
		s.NoError(err)
		s.Equal(configSetting{Key: "apc.page_size", Value: "70", Scope: "project", Set: true}, decodeOne[configSetting](s, stdout))
	})

	s.Run("json and text in a 1.x project that does not set it say so, without falling back", func() {
		s.initConfigFiles("page_size: 50\n", "show_warnings: false\n")
		s.with1xProjectOnDisk("show_warnings: false\n")
		stdout, _, err := runConfigCmd("get", "apc.page_size", "-o", "json")
		s.NoError(err)
		s.Equal(configSetting{Key: "apc.page_size", Value: "", Scope: "project", Set: false}, decodeOne[configSetting](s, stdout))

		stdout, _, err = runConfigCmd("get", "apc.page_size")
		s.NoError(err)
		s.Equal("apc.page_size: \n", stdout, "the text prints the same empty value")

		stdout, _, err = runConfigCmd("list", "-o", "json")
		s.NoError(err)
		s.Contains(decodeOne[configSettings](s, stdout).Settings, configSetting{Key: "apc.page_size", Value: "50", Scope: "global", Set: true},
			"list is where the value in effect is")
	})

	s.Run("json fails as one error object for a setting that does not exist", func() {
		s.initConfigFiles("page_size: 50\n", "")
		stdout, stderr, err := runConfigCmd("get", "nope", "-g", "-o", "json")
		s.ErrorIs(err, errInvalidConfigPath)
		s.Empty(stderr)
		failure := decodeOne[cliout.ErrorObject](s, stdout)
		s.Equal(2, failure.Code)
		s.Equal(cliout.KindUsage, failure.Kind)
	})
}

func (s *CmdSuite) TestConfigSetOutput() {
	s.Run("text is the line it always printed", func() {
		s.initConfigFiles("page_size: 50\n", "")
		stdout, _, err := runConfigCmd("set", "apc.page_size", "60", "-g")
		s.NoError(err)
		s.Equal("Setting apc.page_size to 60 successfully\n\n", stdout)
	})

	s.Run("json publishes the setting as set", func() {
		s.initConfigFiles("page_size: 50\n", "")
		stdout, stderr, err := runConfigCmd("set", "apc.page_size", "60", "-g", "-o", "json")
		s.NoError(err)
		s.Empty(stderr)
		s.Equal(configSetting{Key: "apc.page_size", Value: "60", Scope: "global", Set: true}, decodeOne[configSetting](s, stdout))
		s.Equal("60", config.CFG.PageSize.GetHomeString(), "and wrote it")
	})

	s.Run("json in a 1.x project sets the project's value", func() {
		s.initConfigFiles("page_size: 50\n", "page_size: 70\n")
		s.with1xProjectOnDisk("page_size: 70\n")
		stdout, _, err := runConfigCmd("set", "apc.page_size", "80", "-o", "json")
		s.NoError(err)
		s.Equal(configSetting{Key: "apc.page_size", Value: "80", Scope: "project", Set: true}, decodeOne[configSetting](s, stdout))
		s.Equal("80", config.CFG.PageSize.GetProjectString())
		s.Equal("50", config.CFG.PageSize.GetHomeString(), "the global value is untouched")
	})

	s.Run("json refuses a removed setting as a usage error", func() {
		s.initConfigFiles("page_size: 50\n", "")
		stdout, stderr, err := runConfigCmd("set", "dev.mode", "standalone", "-g", "-o", "json")
		s.Error(err)
		s.Empty(stderr)
		failure := decodeOne[cliout.ErrorObject](s, stdout)
		s.Equal(cliout.KindUsage, failure.Kind)
		s.Equal(2, failure.Code)
	})
}

func (s *CmdSuite) TestConfigListOutput() {
	s.Run("json lists every setting under settings, by key", func() {
		s.initConfigFiles("page_size: 50\ncontexts:\n  a:\n    token: t0ken-value\ncloud:\n  api:\n    token: t0ken-value\n", "")
		stdout, stderr, err := runConfigCmd("list", "-g", "-o", "json")
		s.NoError(err)
		s.Empty(stderr)
		list := decodeOne[configSettings](s, stdout)
		s.Contains(list.Settings, configSetting{Key: "apc.page_size", Value: "50", Scope: "global", Set: true})
		s.Contains(list.Settings, configSetting{Key: "apc.show_warnings", Value: "true", Scope: "default"})
		keys := make([]string, 0, len(list.Settings))
		for _, setting := range list.Settings {
			keys = append(keys, setting.Key)
		}
		s.True(slices.IsSorted(keys), "by key: %v", keys)
		// Only listed settings, and nothing get and set refuse (a removed or
		// renamed 1.x name), so list and get agree.
		for _, key := range keys {
			s.True(config.CFGStrMap[key].Listed(), key)
			_, refused := config.RemovedSetting(key)
			s.False(refused, key)
		}
		s.NotContains(keys, config.CFG.Contexts.Path)
		s.NotContains(keys, config.CFG.AstroLocalCoreURL.Path)
		s.NotContains(stdout, "t0ken-value")
	})

	s.Run("json in a 1.x project reads each value where commands run there do", func() {
		s.initConfigFiles("page_size: 50\n", "page_size: 70\n")
		s.with1xProjectOnDisk("page_size: 70\n")
		stdout, _, err := runConfigCmd("list", "-o", "json")
		s.NoError(err)
		list := decodeOne[configSettings](s, stdout)
		s.Contains(list.Settings, configSetting{Key: "apc.page_size", Value: "70", Scope: "project", Set: true})
	})
}
