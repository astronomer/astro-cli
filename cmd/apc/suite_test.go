package apc

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type Suite struct {
	suite.Suite
	restoreTerminal func()
}

func TestSoftware(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) SetupSuite() {
	// The suite pins the layout a person reads; the piped form has its own tests.
	s.restoreTerminal = input.ForceTerminalWriters()
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
}

func (s *Suite) SetupTest() {
	// Reset the version once this is torn down
	houstonVersion = "0.34.0"
	DagsOnlyDeploy = deploy.DagsOnlyDeploy
}

func (s *Suite) SetupSubTest() {
	DagsOnlyDeploy = deploy.DagsOnlyDeploy
}

func (s *Suite) TearDownSuite() {
	s.restoreTerminal()
	DagsOnlyDeploy = deploy.DagsOnlyDeploy
}

var (
	_ suite.SetupAllSuite    = (*Suite)(nil)
	_ suite.SetupTestSuite   = (*Suite)(nil)
	_ suite.TearDownAllSuite = (*Suite)(nil)
)
