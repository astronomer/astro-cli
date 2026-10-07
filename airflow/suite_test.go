package airflow

import (
	"io"
	"os"
	"testing"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/suite"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type Suite struct {
	suite.Suite
	origCmdExec         func(cmd string, stdout, stderr io.Writer, args ...string) error
	origRegistryLogin   func(containerRuntime, server, username, password string, stdout, stderr io.Writer) error
	origGetDockerClient func() (client.APIClient, error)
	origStdout          *os.File
}

var (
	_ suite.SetupAllSuite     = (*Suite)(nil)
	_ suite.SetupTestSuite    = (*Suite)(nil)
	_ suite.TearDownTestSuite = (*Suite)(nil)
	_ suite.TearDownSubTest   = (*Suite)(nil)
)

func TestAirflow(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) SetupSuite() {
	s.origCmdExec = cmdExec
	s.origRegistryLogin = registryLogin
	s.origGetDockerClient = getDockerClient
	s.origStdout = os.Stdout
}

func (s *Suite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	registryLogin = func(_, _, _, _ string, _, _ io.Writer) error { return nil }
}

func (s *Suite) TearDownTest() {
	cmdExec = s.origCmdExec
	registryLogin = s.origRegistryLogin
	getDockerClient = s.origGetDockerClient
}

func (s *Suite) TearDownSubTest() {
	os.Stdout = s.origStdout
}
