package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) useWorkingPath(dir string) {
	orig := config.WorkingPath
	config.WorkingPath = dir
	s.T().Cleanup(func() { config.WorkingPath = orig })
}

func (s *CmdSuite) useManifestProject() {
	dir := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"p\"\nversion = \"0.1.0\"\n\n[tool.astro]\n"), 0o600))
	s.useWorkingPath(dir)
}

func (s *CmdSuite) TestConfigRootCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("config")
	s.NoError(err)
	s.Contains(output, "astro config")
}

func (s *CmdSuite) TestConfigGetCommandSuccess() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "get", "apc.show_warnings", "-g")
	s.NoError(err)
}

func (s *CmdSuite) TestConfigGetCommandFailure() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "get", "-g", "test")
	s.ErrorIs(err, errInvalidConfigPath)
	s.True(cliout.IsUsage(err), "an unknown setting is a usage error, as an unknown flag is")
	s.EqualError(err, `unknown setting: "test". Run astro config list to see every setting.`)

	// Outside a project and without -g, an unknown name is still an unknown
	// name, not advice to retry with -g, which would only fail again.
	s.useWorkingPath(s.T().TempDir())
	_, err = executeCommand("config", "get", "test")
	s.ErrorIs(err, errInvalidConfigPath)
	s.Equal(2, cliout.ExitCode(context.Background(), err))
	s.NotContains(err.Error(), "outside of a project directory")

	_, err = executeCommand("config", "get", "upgrade_message")
	s.Error(err)
	s.Contains(err.Error(), "You are attempting to get a project config outside of a project directory")
	// The suggested command has to be runnable. It used to interpolate the
	// argument placeholder alongside the real argument.
	s.Contains(err.Error(), "astro config get upgrade_message -g")
	s.NotContains(err.Error(), "[setting-name]")
}

func (s *CmdSuite) TestConfigSetOutsideProjectSuggestsTheValueToo() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useWorkingPath(s.T().TempDir())
	_, err := executeCommand("config", "set", "apc.show_warnings", "false")
	s.ErrorContains(err, "outside of a project directory")
	s.ErrorContains(err, "astro config set apc.show_warnings false -g")
}

func (s *CmdSuite) TestConfigInAManifestProjectPointsAtGlobalAndPyproject() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useManifestProject()

	_, err := executeCommand("config", "set", "apc.show_warnings", "false")
	s.Error(err)
	s.NotContains(err.Error(), "outside of a project directory")
	s.Contains(err.Error(), "pyproject.toml under [tool.astro]")
	s.Contains(err.Error(), "astro config set apc.show_warnings false -g")
	s.Equal("true", config.CFG.ShowWarnings.GetHomeString())

	_, err = executeCommand("config", "get", "deploy.git_metadata")
	s.Error(err)
	s.Contains(err.Error(), "pyproject.toml under [tool.astro]")
	s.Contains(err.Error(), "astro config get deploy.git_metadata -g")

	_, err = executeCommand("config", "set", "apc.show_warnings", "false", "-g")
	s.NoError(err)
	s.Equal("false", config.CFG.ShowWarnings.GetHomeString())
}

func (s *CmdSuite) TestConfigSuggestionQuotesAValueWithSpaces() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useManifestProject()
	_, err := executeCommand("config", "set", "internal.astro.localhost.core_url", "http://my host")
	s.ErrorContains(err, "astro config set internal.astro.localhost.core_url 'http://my host' -g")
}

func (s *CmdSuite) TestConfigIn1xProjectWithPyprojectKeepsProjectScope() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useManifestProject()
	s.Require().NoError(os.Mkdir(filepath.Join(config.WorkingPath, config.ConfigDir), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(config.WorkingPath, config.ConfigDir, config.ConfigFileNameWithExt), nil, 0o600))
	_, err := executeCommand("config", "get", "apc.page_size")
	s.NoError(err)
}

// initConfigFiles loads config from an in-memory home config and, when
// project is not empty, a 1.x project config in a fresh working directory. It
// returns the filesystem, for a test that reads back what was written.
func (s *CmdSuite) initConfigFiles(home, project string) afero.Fs {
	fs := afero.NewMemMapFs()
	projectDir := filepath.Join(s.T().TempDir(), "proj")
	s.useWorkingPath(projectDir)
	s.Require().NoError(afero.WriteFile(fs, config.HomeConfigFile, []byte(home), 0o600))
	if project != "" {
		s.Require().NoError(afero.WriteFile(fs, filepath.Join(projectDir, config.ConfigDir, config.ConfigFileNameWithExt), []byte(project), 0o600))
	}
	config.InitConfig(fs)
	s.T().Cleanup(func() { testUtil.InitTestConfig(testUtil.LocalPlatform) })
	globalFlag = false
	s.T().Cleanup(func() { globalFlag = false })
	return fs
}

func (s *CmdSuite) TestConfigListShowsValueAndScope() {
	s.initConfigFiles("page_size: 50\ncontexts:\n  a:\n    token: t0ken-value\ncloud:\n  api:\n    token: t0ken-value\n", "")

	buf := new(bytes.Buffer)
	s.NoError(configList(textTo(buf)))
	out := buf.String()
	s.Regexp(`(?m)^\s*apc\.page_size\s+50\s+global\s*$`, out, "set under its 1.x name")
	s.Regexp(`(?m)^\s*apc\.show_warnings\s+true\s+default\s*$`, out)
	s.NotContains(out, "contexts")
	s.NotContains(out, "t0ken-value")
	s.NotContains(out, "postgres", "a removed setting is not listed")
	s.NotContains(out, "internal.", "nor an internal override")
	s.NotRegexp(`(?m)^\s*page_size\s`, out, "nor a 1.x name")
}

func (s *CmdSuite) TestConfigListReadsA1xProjectUnlessGlobal() {
	s.initConfigFiles("page_size: 50\n", "page_size: 70\n")

	buf := new(bytes.Buffer)
	s.NoError(configList(textTo(buf)))
	s.Regexp(`(?m)^\s*apc\.page_size\s+70\s+project\s*$`, buf.String())

	globalFlag = true
	buf.Reset()
	s.NoError(configList(textTo(buf)))
	s.Regexp(`(?m)^\s*apc\.page_size\s+50\s+global\s*$`, buf.String())
}

// A bare `astro config set` panicked: ensureGlobalFlag runs as a
// PersistentPreRunE, which cobra calls before the subcommand checks its own
// arguments, and it indexed args[0] unguarded.
func (s *CmdSuite) TestConfigSetWithNoArgumentsReportsArity() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "set")
	s.ErrorIs(err, errInvalidSetArgs)
	s.True(cliout.IsUsage(err), "the wrong number of arguments is a usage error, exit 2")
}

func (s *CmdSuite) TestConfigSetCommandFailure() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "set", "test", "testing", "-g")
	s.Error(err)

	_, err = executeCommand("config", "set", "test", "-g")
	s.ErrorIs(err, errInvalidSetArgs)
	s.True(cliout.IsUsage(err), "the wrong number of arguments is a usage error, exit 2")
}

// Outside a project and without -g, the group's pre-run would refuse the
// scope before configSet saw the count, as a plain error exiting 1. The count
// is checked first, as Args, so a wrong one is a usage error wherever it is.
func (s *CmdSuite) TestConfigSetWithOneArgumentOutsideAProjectIsAUsageError() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useWorkingPath(s.T().TempDir())

	for _, args := range [][]string{{"config", "set", "webserver.port"}, {"config", "set", "a", "b", "c"}} {
		_, err := executeCommand(args...)
		s.ErrorIs(err, errInvalidSetArgs, args)
		s.True(cliout.IsUsage(err), "the wrong number of arguments is a usage error, exit 2: %v", args)
		s.Equal(2, cliout.ExitCode(context.Background(), err), args)
	}
}

func (s *CmdSuite) TestConfigSetCommandSuccess() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "set", "-g", "apc.page_size", "30")
	s.NoError(err)
}

// The 1.x settings nothing in v2 reads are refused, in every scope, as a
// usage error naming what replaces them, and nothing is written.
func (s *CmdSuite) TestConfigSetRefusesRemovedKeys() {
	for _, tc := range []struct {
		key, value, want string
	}{
		{"dev.mode", "standalone", "astro local start --docker"},
		{"proxy.port", "6599", "listens on 6563"},
		{"api-server.port", "8099", "astro local start --port <port>"},
		{"webserver.port", "8099", "astro local start --port <port>"},
		{"project.deployment", "clx-old", "as the argument or with --deployment"},
		{"postgres.user", "me", "runs its own metadata database"},
		{"project.name", "p", "[project] name in its pyproject.toml"},
		{"skip_parse", "true", "astro local check"},
		{"cloud.api.token", "t0ken", "ASTRO_API_TOKEN"},
		// Renamed: refused, naming the new name, as a renamed flag is.
		{"page_size", "30", "was removed in Astro CLI v2: use apc.page_size"},
		{"auto_select", "true", "was removed in Astro CLI v2: use deployment.auto_select"},
		{"local.core", "http://x", "was removed in Astro CLI v2: use internal.astro.localhost.core_url"},
		{"cloud.api.port", "8443", "was removed in Astro CLI v2: use internal.apc.api.port"},
		{"cloud.api.ws_protocol", "ws", "was removed in Astro CLI v2: use internal.apc.api.ws_protocol"},
		{"local.houston", "http://x", "was removed in Astro CLI v2: use internal.apc.houston_url"},
		{"machine.cpu", "4", "podman machine set"},
		{"beta.remote_cmd", "true", "beta flag"},
	} {
		for _, scope := range []string{"outside a project", "global", "manifest project"} {
			s.Run(tc.key+" "+scope, func() {
				fs := s.initConfigFiles("config_version: 2\npage_size: 50\n", "")
				args := []string{"config", "set", tc.key, tc.value}
				switch scope {
				case "outside a project":
					s.useWorkingPath(s.T().TempDir())
				case "global":
					args = append(args, "-g")
				case "manifest project":
					s.useManifestProject()
				}

				_, err := executeCommand(args...)
				s.Require().Error(err)
				s.True(cliout.IsUsage(err), "want a usage error, got %v", err)
				s.ErrorContains(err, tc.key+" was removed in Astro CLI v2")
				s.ErrorContains(err, tc.want)
				after, err := afero.ReadFile(fs, config.HomeConfigFile)
				s.Require().NoError(err)
				s.Equal("config_version: 2\npage_size: 50\n", string(after), "the refused value was written")
			})
		}
	}
}

// get refuses a removed key as set does, rather than show a value nothing
// reads.
func (s *CmdSuite) TestConfigGetRefusesRemovedKeys() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useWorkingPath(s.T().TempDir())
	_, err := executeCommand("config", "get", "project.deployment")
	s.Require().Error(err)
	s.True(cliout.IsUsage(err), "want a usage error, got %v", err)
	s.ErrorContains(err, "project.deployment was removed in Astro CLI v2")
}

// configSet guards the write on its own, for a caller that skips the pre-run.
func (s *CmdSuite) TestConfigSetNeverWritesARemovedKey() {
	fs := s.initConfigFiles("config_version: 2\npage_size: 50\n", "")
	globalFlag = true
	err := configSet(newConfigSetCmd(nil), []string{"dev.mode", "standalone"})
	s.True(cliout.IsUsage(err), "want a usage error, got %v", err)
	after, err := afero.ReadFile(fs, config.HomeConfigFile)
	s.Require().NoError(err)
	s.Equal("config_version: 2\npage_size: 50\n", string(after))
}

// A save writes back what the file held, the settings only Astro CLI 1.x
// reads included, plus the one change: never the defaults, which would pin
// every one of them at this build's value, and never a removed key.
func (s *CmdSuite) TestConfigSetWritesOnlyTheChangeAndKeeps1xSettings() {
	fs := s.initConfigFiles("postgres:\n  user: me\nwebserver:\n  port: \"8081\"\ncontext: localhost\n", "")

	_, err := executeCommand("config", "set", "apc.show_warnings", "false", "-g")
	s.Require().NoError(err)
	_, err = executeCommand("config", "set", "apc.page_size", "30", "-g")
	s.Require().NoError(err)

	got, err := afero.ReadFile(fs, config.HomeConfigFile)
	s.Require().NoError(err)
	s.YAMLEq(`
apc:
  page_size: 30
  show_warnings: false
config_version: 2
context: localhost
postgres:
  user: me
webserver:
  port: "8081"
`, string(got))
	s.Equal(30, config.CFG.PageSize.GetInt())
	s.False(config.CFG.ShowWarnings.GetBool())
	s.True(config.CFG.UpgradeMessage.GetBool(), "an unset setting reads as its default")
	s.Equal("default", config.CFG.UpgradeMessage.HomeScope())
}

// A name that is no setting fails as an unknown flag does, exit 2, naming the
// setting it is most likely a typo of when one is close.
func (s *CmdSuite) TestConfigUnknownSettingSuggestsTheClosest() {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"a typo", []string{"config", "set", "apc.page_sise", "30", "-g"}, " Did you mean apc.page_size? "},
		{"a typo", []string{"config", "get", "deploy.git_metdata", "-g"}, " Did you mean deploy.git_metadata? "},
		{"an accented letter is one edit", []string{"config", "get", "apc.page_sizé", "-g"}, " Did you mean apc.page_size? "},
		{
			"every setting under a partial name",
			[]string{"config", "get", "telemetry", "-g"},
			" Did you mean one of: telemetry.anonymous_id, telemetry.enabled, telemetry.notice_shown? ",
		},
		{"a typo of a renamed 1.x name suggests the current one", []string{"config", "get", "page_sise", "-g"}, " Did you mean apc.page_size? "},
		{
			"a typo of a removed 1.x name says it was removed",
			[]string{"config", "set", "webserver.prot", "8081", "-g"},
			"Did you mean webserver.port? It was removed in Astro CLI v2: nothing reads it. Pick the local Airflow port",
		},
		{"outside a project without -g", []string{"config", "get", "apc.page_sise"}, " Did you mean apc.page_size? "},
	} {
		s.Run(tc.name+": "+strings.Join(tc.args, " "), func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			s.useWorkingPath(s.T().TempDir())
			_, err := executeCommand(tc.args...)
			s.ErrorIs(err, errInvalidConfigPath)
			s.Equal(2, cliout.ExitCode(context.Background(), err))
			s.ErrorContains(err, tc.want)
		})
	}

	// Never a name list leaves out: contexts, whose set would replace every
	// saved login, nor the internal.* overrides. And not every setting for an
	// empty name, which is a prefix of all of them.
	s.Run("never contexts", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		_, err := executeCommand("config", "set", "contxts", "x", "-g")
		s.ErrorIs(err, errInvalidConfigPath)
		s.NotContains(err.Error(), "contexts")
	})
	for _, args := range [][]string{
		{"config", "set", "internal.astro.localhost.core", "x", "-g"},
		{"config", "get", "", "-g"},
		{"config", "get", "xyzzy", "-g"},
	} {
		s.Run("no suggestion: "+strings.Join(args, " "), func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			_, err := executeCommand(args...)
			s.ErrorIs(err, errInvalidConfigPath)
			s.NotContains(err.Error(), "Did you mean")
		})
	}
}
