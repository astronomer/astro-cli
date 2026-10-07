package deploy

import (
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/input"
)

// An image tag that is not recommended is a question --yes answers. Under a
// guard that refuses questions, without it, the refusal names --yes.
func (s *Suite) TestValidateRuntimeVersionAnsweredByYes() {
	config.InitConfig(s.fsForLocalConfig)
	deploymentInfo := &houston.Deployment{ClusterID: "c", DesiredAirflowVersion: "2.0.0"}
	s.houstonMock.On("GetDeploymentConfig", nil).Return(&houston.DeploymentConfig{AirflowImages: mockAirflowImageList}, nil)
	s.houstonMock.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{}, nil)
	defer input.SetGuard(func() string { return "with --output json it cannot" })()

	err := validateRuntimeVersion(s.houstonMock, "2.0.0-custom", deploymentInfo, Options{})
	s.ErrorContains(err, "--yes")
	s.NoError(validateRuntimeVersion(s.houstonMock, "2.0.0-custom", deploymentInfo, Options{Yes: true}))
}
