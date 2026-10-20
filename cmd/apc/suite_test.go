package apc

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/apc/deploy"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/pkg/git"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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
	DagsOnlyDeploy = deploy.DagsOnlyDeploy
	// The real deploy steps, so no test inherits another's stubs.
	EnsureProjectDir = utils.EnsureProjectDir
	DeployAirflowImage = deploy.Airflow
	// A clean checkout, whatever state the one the tests run in is in.
	hasUncommittedChanges = func(string) bool { return false }
}

func (s *Suite) SetupSubTest() {
	DagsOnlyDeploy = deploy.DagsOnlyDeploy
}

func (s *Suite) TearDownSuite() {
	DagsOnlyDeploy = deploy.DagsOnlyDeploy
	UpdateDeploymentImage = deploy.UpdateDeploymentImage
	EnsureProjectDir = utils.EnsureProjectDir
	DeployAirflowImage = deploy.Airflow
	hasUncommittedChanges = git.HasUncommittedChanges
}

var (
	_ suite.SetupAllSuite    = (*Suite)(nil)
	_ suite.SetupTestSuite   = (*Suite)(nil)
	_ suite.TearDownAllSuite = (*Suite)(nil)
)
