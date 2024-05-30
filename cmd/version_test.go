package cmd

import (
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) TestVersionRootCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// Locally we are not support version, that's why we see err
	expectedOut := ""
	output, err := executeCommand("version")
	s.Equal(expectedOut, output, err)
}
