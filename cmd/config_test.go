package cmd

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) useWorkingPath(dir string) {
	orig := config.WorkingPath
	config.WorkingPath = dir
	s.T().Cleanup(func() { config.WorkingPath = orig })
}

func (s *CmdSuite) useV2Project() {
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

func (s *CmdSuite) TestConfigInV2ProjectPointsAtGlobalAndPyproject() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useV2Project()

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
	s.useV2Project()
	_, err := executeCommand("config", "set", "dev.build_secrets", "id=a,src=/my path")
	s.ErrorContains(err, "astro config set dev.build_secrets 'id=a,src=/my path' -g")
}

func (s *CmdSuite) TestConfigInV1ProjectWithPyprojectKeepsProjectScope() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.useV2Project()
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
	s.NoError(configList(buf))
	out := buf.String()
	s.Regexp(`(?m)^\s*page_size\s+50\s+global\s*$`, out)
	s.Regexp(`(?m)^\s*show_warnings\s+true\s+default\s*$`, out)
	s.NotContains(out, "contexts")
	s.NotContains(out, "t0ken-value")
	s.NotContains(out, config.CFG.PostgresPassword.Path)
}

func (s *CmdSuite) TestConfigListReadsAV1ProjectUnlessGlobal() {
	s.initConfigFiles("page_size: 50\n", "page_size: 70\n")

	buf := new(bytes.Buffer)
	s.NoError(configList(buf))
	s.Regexp(`(?m)^\s*page_size\s+70\s+project\s*$`, buf.String())

	globalFlag = true
	buf.Reset()
	s.NoError(configList(buf))
	s.Regexp(`(?m)^\s*page_size\s+50\s+global\s*$`, buf.String())
}

// A bare `astro config set` panicked: ensureGlobalFlag runs as a
// PersistentPreRunE, which cobra calls before the subcommand checks its own
// arguments, and it indexed args[0] unguarded.
func (s *CmdSuite) TestConfigSetWithNoArgumentsReportsArity() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "set")
	s.ErrorIs(err, errInvalidSetArgs)
}

func (s *CmdSuite) TestConfigSetCommandFailure() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "set", "test", "testing", "-g")
	s.Error(err)

	_, err = executeCommand("config", "set", "test", "-g")
	s.ErrorIs(err, errInvalidSetArgs)
}

func (s *CmdSuite) TestConfigSetCommandSuccess() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	_, err := executeCommand("config", "set", "-g", "project.name", "testing")
	s.NoError(err)
}
