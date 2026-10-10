package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

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
	_, err := executeCommand("config", "get", "project.name", "-g")
	s.NoError(err)
}

func (s *CmdSuite) TestConfigGetCommandFailure() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "get", "-g", "test")
	s.Error(err)
	s.EqualError(err, errInvalidConfigPath.Error())

	_, err = executeCommand("config", "get", "test")
	s.Error(err)
	s.Contains(err.Error(), "You are attempting to get a project config outside of a project directory")
	// The suggested command has to be runnable. It used to interpolate the
	// argument placeholder alongside the real argument.
	s.Contains(err.Error(), "astro config get test -g")
	s.NotContains(err.Error(), "[setting-name]")
}

func (s *CmdSuite) TestConfigSetOutsideProjectSuggestsTheValueToo() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useWorkingPath(s.T().TempDir())
	_, err := executeCommand("config", "set", "show_warnings", "false")
	s.ErrorContains(err, "outside of a project directory")
	s.ErrorContains(err, "astro config set show_warnings false -g")
}

func (s *CmdSuite) TestConfigInAManifestProjectPointsAtGlobalAndPyproject() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useManifestProject()

	_, err := executeCommand("config", "set", "show_warnings", "false")
	s.Error(err)
	s.NotContains(err.Error(), "outside of a project directory")
	s.Contains(err.Error(), "pyproject.toml under [tool.astro]")
	s.Contains(err.Error(), "astro config set show_warnings false -g")
	s.Equal("true", config.CFG.ShowWarnings.GetHomeString())

	_, err = executeCommand("config", "get", "deploy.git_metadata")
	s.Error(err)
	s.Contains(err.Error(), "pyproject.toml under [tool.astro]")
	s.Contains(err.Error(), "astro config get deploy.git_metadata -g")

	_, err = executeCommand("config", "set", "show_warnings", "false", "-g")
	s.NoError(err)
	s.Equal("false", config.CFG.ShowWarnings.GetHomeString())
}

func (s *CmdSuite) TestConfigSuggestionQuotesAValueWithSpaces() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useManifestProject()
	_, err := executeCommand("config", "set", "dev.build_secrets", "id=a,src=/my path")
	s.ErrorContains(err, "astro config set dev.build_secrets 'id=a,src=/my path' -g")
}

func (s *CmdSuite) TestConfigIn1xProjectWithPyprojectKeepsProjectScope() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useManifestProject()
	s.Require().NoError(os.Mkdir(filepath.Join(config.WorkingPath, config.ConfigDir), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(config.WorkingPath, config.ConfigDir, config.ConfigFileNameWithExt), nil, 0o600))
	_, err := executeCommand("config", "get", "page_size")
	s.NoError(err)
}

// initConfigFiles loads config from an in-memory home config and, when
// project is not empty, a 1.x project config in a fresh working directory.
func (s *CmdSuite) initConfigFiles(home, project string) {
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
}

func (s *CmdSuite) TestConfigListShowsValueAndScope() {
	s.initConfigFiles("page_size: 50\ncontexts:\n  a:\n    token: t0ken-value\ncloud:\n  api:\n    token: t0ken-value\n", "")

	buf := new(bytes.Buffer)
	s.NoError(configList(textTo(buf)))
	out := buf.String()
	s.Regexp(`(?m)^\s*page_size\s+50\s+global\s*$`, out)
	s.Regexp(`(?m)^\s*show_warnings\s+true\s+default\s*$`, out)
	s.NotContains(out, "contexts")
	s.NotContains(out, "t0ken-value")
	s.NotContains(out, config.CFG.PostgresPassword.Path)
}

func (s *CmdSuite) TestConfigListReadsA1xProjectUnlessGlobal() {
	s.initConfigFiles("page_size: 50\n", "page_size: 70\n")

	buf := new(bytes.Buffer)
	s.NoError(configList(textTo(buf)))
	s.Regexp(`(?m)^\s*page_size\s+70\s+project\s*$`, buf.String())

	globalFlag = true
	buf.Reset()
	s.NoError(configList(textTo(buf)))
	s.Regexp(`(?m)^\s*page_size\s+50\s+global\s*$`, buf.String())
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
	_, err := executeCommand("config", "set", "-g", "project.name", "testing")
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
	} {
		for _, scope := range []string{"outside a project", "global", "manifest project"} {
			s.Run(tc.key+" "+scope, func() {
				testUtil.InitTestConfig(testUtil.LocalPlatform)
				args := []string{"config", "set", tc.key, tc.value}
				switch scope {
				case "outside a project":
					s.useWorkingPath(s.T().TempDir())
				case "global":
					args = append(args, "-g")
				case "manifest project":
					s.useManifestProject()
				}
				before := config.CFGStrMap[tc.key].GetHomeString()

				_, err := executeCommand(args...)
				s.Require().Error(err)
				s.True(cliout.IsUsage(err), "want a usage error, got %v", err)
				s.ErrorContains(err, "`"+tc.key+"` was removed in Astro CLI v2")
				s.ErrorContains(err, tc.want)
				s.Equal(before, config.CFGStrMap[tc.key].GetHomeString(), "the refused value was written")
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
	s.ErrorContains(err, "`project.deployment` was removed in Astro CLI v2")
}

// configSet guards the write on its own, for a caller that skips the pre-run.
func (s *CmdSuite) TestConfigSetNeverWritesARemovedKey() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	globalFlag = true
	s.T().Cleanup(func() { globalFlag = false })
	before := config.CFG.DevMode.GetHomeString()
	err := configSet(newConfigSetCmd(nil), []string{"dev.mode", "standalone"})
	s.True(cliout.IsUsage(err), "want a usage error, got %v", err)
	s.Equal(before, config.CFG.DevMode.GetHomeString())
}
