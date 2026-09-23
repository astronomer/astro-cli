package apc

import (
	"bytes"
	"os"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestUserRootCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	testUtil.SetupOSArgsForGinkgo()
	buf := new(bytes.Buffer)
	userCmd := newUserCmd(os.Stdout)
	userCmd.SetOut(buf)
	_, err := userCmd.ExecuteC()
	s.NoError(err)
	s.Contains(buf.String(), "A user represents a human who has authenticated with the APC platform")
	s.Contains(buf.String(), "create")
}
