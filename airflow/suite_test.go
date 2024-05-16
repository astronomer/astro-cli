package airflow

import (
	"io"
	"testing"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
  origCmdExec func(cmd string, stdout, stderr io.Writer, args ...string) error
  origGetDockerClient func() (client.APIClient, error)
}

var _ suite.SetupAllSuite = (*Suite)(nil)
var _ suite.TearDownTestSuite = (*Suite)(nil)

func TestAirflow(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) SetupSuite() {
  s.origCmdExec = cmdExec
  s.origGetDockerClient = getDockerClient
}

func (s *Suite) TearDownTest() {
  cmdExec = s.origCmdExec
  getDockerClient = s.origGetDockerClient
}
