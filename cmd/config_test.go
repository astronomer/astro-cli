package cmd

import (
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

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
