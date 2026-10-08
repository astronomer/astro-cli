package deploy

import (
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/airflow"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// A custom --image-name pushed to the APC registry goes to
// <registry>/<release>/airflow:<nextTag>, whatever its own tag. Houston
// deploys only a push to <release>/airflow, skips a push tagged "latest",
// and does not roll out a tag it has deployed before. Pushed under the
// user's tag, "latest" or a reused tag would deploy nothing.
func (s *Suite) TestCustomImageToTheAPCRegistryTakesTheNextTag() {
	config.InitConfig(s.fsForDockerConfig)
	s.houstonMock.On("GetDeploymentConfig", nil).Return(&houston.DeploymentConfig{AirflowImages: mockAirflowImageList}, nil)
	s.houstonMock.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{}, nil)
	// Below 1.0.0 the registry is registry.<domain>, with no per-Deployment
	// login to mock.
	s.houstonMock.On("GetPlatformVersion", mock.Anything).Return("0.30.0", nil)

	for _, c := range []struct {
		image, nextTag, want string
	}{
		{"my-image:v7", "deploy-3", "registry.example.com/my-release/airflow:deploy-3"},
		{"my-image:latest", "deploy-4", "registry.example.com/my-release/airflow:deploy-4"},
		// The same image tag deployed again still rolls out: a new tag.
		{"my-image:v7", "deploy-5", "registry.example.com/my-release/airflow:deploy-5"},
	} {
		var pushedTo string
		imageHandlerInit = func(image string) airflow.ImageHandler {
			s.mockImageHandler.On("TagLocalImage", c.image).Return(nil).Once()
			s.mockImageHandler.On("GetLabel", "", airflow.RuntimeImageLabel).Return("12.2.0", nil).Once()
			s.mockImageHandler.On("Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				pushedTo = args.String(0)
			}).Return("", nil).Once()
			return s.mockImageHandler
		}

		got, err := buildPushDockerImage(s.houstonMock, &config.Context{}, mockDeployment, "my-release", "./testfiles/", c.nextTag, "example.com", "", false, false, description, c.image, Options{})
		s.NoError(err, c.image)
		s.Equal(c.want, pushedTo, c.image)
		s.Equal(c.want, got)
	}
}
