package deploy

import (
	"bytes"
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// apcDeployment is the Deployment these tests push for. Its registry is the
// one below Houston 1.0, registry.<domain>, so there is no per-Deployment
// login to mock.
var apcDeployment = &houston.Deployment{ID: "dep-id", ReleaseName: "my-release"}

// mockImageEngine is an imageEngine that records what a deploy asked of the
// container engine and answers as told.
type mockImageEngine struct{ mock.Mock }

func (m *mockImageEngine) Tag(_ context.Context, src, dst string) error {
	return m.Called(src, dst).Error(0)
}

func (m *mockImageEngine) Label(_ context.Context, image, label string) (string, error) {
	args := m.Called(image, label)
	return args.String(0), args.Error(1)
}

func (m *mockImageEngine) Push(_ context.Context, req imagebuild.PushRequest) (string, error) { //nolint:gocritic // the signature imageEngine takes
	args := m.Called(req)
	return args.String(0), args.Error(1)
}

// The image a deploy pushes is the custom image tagged as the release's
// <release>/airflow:latest, and its labels are read off that tag. To the APC
// registry it is pushed with the context's token as "user", logged in to as
// the push starts, its progress where the deploy's goes; to a BYO registry
// with the credential the engine already holds, by digest under sha_as_tag.
func (s *Suite) TestTagPushDockerImageAsksTheEngine() {
	config.InitConfig(s.fsForDockerConfig)
	s.houstonMock.On("GetDeploymentConfig", nil).Return(&houston.DeploymentConfig{AirflowImages: mockAirflowImageList}, nil)
	s.houstonMock.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{}, nil)

	s.Run("the APC registry", func() {
		s.houstonMock.On("GetDeploymentConfig", nil).Return(&houston.DeploymentConfig{AirflowImages: mockAirflowImageList}, nil)
		s.houstonMock.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{}, nil)
		s.houstonMock.On("GetPlatformVersion", mock.Anything).Return("0.30.0", nil)
		s.mockImageEngine.On("Tag", "my-image:v1", "my-release/airflow:latest").Return(nil).Once()
		s.mockImageEngine.On("Label", "my-release/airflow:latest", airflowrt.RuntimeVersionLabel).Return("12.2.0", nil).Once()
		var progress bytes.Buffer
		s.mockImageEngine.On("Push", imagebuild.PushRequest{
			Source:   "my-release/airflow:latest",
			Image:    "registry.example.com/my-release/airflow:deploy-1",
			Username: "user",
			Secret:   "ctx-token",
			Out:      &progress,
		}).Return("", nil).Once()

		got, err := tagPushDockerImage(s.houstonMock, &config.Context{Token: "ctx-token"}, apcDeployment, "my-release", "deploy-1", "example.com", "", false, "my-image:v1", Options{Progress: &progress})
		s.NoError(err)
		s.Equal("registry.example.com/my-release/airflow:deploy-1", got)
	})

	s.Run("a BYO registry by digest", func() {
		s.houstonMock.On("GetDeploymentConfig", nil).Return(&houston.DeploymentConfig{AirflowImages: mockAirflowImageList}, nil)
		s.houstonMock.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{}, nil)
		config.CFG.ShaAsTag.SetHomeString("true")
		defer config.CFG.ShaAsTag.SetHomeString("false")
		s.mockImageEngine.On("Tag", "my-image:v1", "my-release/airflow:latest").Return(nil).Once()
		s.mockImageEngine.On("Label", "my-release/airflow:latest", airflowrt.RuntimeVersionLabel).Return("12.2.0", nil)
		s.mockImageEngine.On("Label", "my-release/airflow:latest", airflowImageLabel).Return("2.9.0", nil).Once()
		var progress bytes.Buffer
		s.mockImageEngine.On("Push", imagebuild.PushRequest{
			Source: "my-release/airflow:latest",
			Image:  "byo.example.com/team/repo:v1",
			Digest: true,
			Out:    &progress,
		}).Return("sha256:abc", nil).Once()
		s.houstonMock.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{
			ReleaseName: "my-release", Image: "byo.example.com/team/repo@sha256:abc", AirflowVersion: "2.9.0", RuntimeVersion: "12.2.0",
		}).Return(nil, nil).Once()

		got, err := tagPushDockerImage(s.houstonMock, &config.Context{Token: "ctx-token"}, apcDeployment, "my-release", "deploy-1", "example.com", "byo.example.com/team/repo", true, "my-image:v1", Options{Progress: &progress})
		s.NoError(err)
		s.Equal("byo.example.com/team/repo@sha256:abc", got)
	})
}

// A deploy that cannot resolve a container engine fails before it tags.
func (s *Suite) TestTagPushDockerImageWithoutAnEngine() {
	newImageEngine = func() (imageEngine, error) { return nil, errSomeContainerIssue }
	_, err := tagPushDockerImage(s.houstonMock, &config.Context{}, apcDeployment, "my-release", "deploy-1", "example.com", "", false, "my-image:v1", Options{})
	s.ErrorIs(err, errSomeContainerIssue)
}
