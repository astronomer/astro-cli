package registry

import (
	"bytes"
	"os"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestRegistry(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestRegistryCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	deplyCmd := newRegistryCmd(os.Stdout)
	deplyCmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := deplyCmd.ExecuteC()
	s.NoError(err)
	s.Contains(buf.String(), "Astronomer Registry")
}
