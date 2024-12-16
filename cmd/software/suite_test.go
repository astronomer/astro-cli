package software

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/software/deploy"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestSoftware(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) SetupSuite() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
}

func (s *Suite) SetupTest() {
	// Reset the version once this is torn down
	houstonVersion = "0.34.0"
}

func (s *Suite) TearDownSuite() {
	DagsOnlyDeploy = deploy.DagsOnlyDeploy
	UpdateDeploymentImage = deploy.UpdateDeploymentImage
}

var (
	_ suite.SetupAllSuite    = (*Suite)(nil)
	_ suite.SetupTestSuite   = (*Suite)(nil)
	_ suite.TearDownAllSuite = (*Suite)(nil)
)
