package airflow

import (
	"io"
	"os"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
	origCmdExec              func(cmd string, stdout, stderr io.Writer, args ...string) error
	origGetDockerClient      func() (client.APIClient, error)
	origInitSettings         func(id, settingsFile string, envConns map[string]astrocore.EnvironmentObjectConnection, version uint64, connections, variables, pools bool) error
	origCheckWebserverHealth func(url string, timeout time.Duration, component string) error
	origStdout               *os.File
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
	s.origGetDockerClient = getDockerClient
	s.origStdout = os.Stdout
	s.origInitSettings = initSettings
	s.origCheckWebserverHealth = checkWebserverHealth
}

func (s *Suite) SetupTest() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
}

func (s *Suite) TearDownTest() {
	cmdExec = s.origCmdExec
	getDockerClient = s.origGetDockerClient
	initSettings = s.origInitSettings
}

func (s *Suite) TearDownSubTest() {
	os.Stdout = s.origStdout
	checkWebserverHealth = s.origCheckWebserverHealth
}
