package apc

import (
	"errors"
	"fmt"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var errMock = errors.New("test error")

func getTestLogs(component string) []houston.DeploymentLog {
	return []houston.DeploymentLog{
		{
			ID:        "1",
			Component: component,
			CreatedAt: "2019-10-16T21:14:22.105Z",
			Log:       "test",
		},
		{
			ID:        "2",
			Component: component,
			CreatedAt: "2019-10-16T21:14:22.105Z",
			Log:       "second test",
		},
	}
}

func (s *Suite) TestDeploymentLogsRootCommandTriggererEnabled() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	appConfig = &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}

	output, err := execDeploymentCmd("logs", "--help")
	s.NoError(err)
	s.Contains(output, "astro deployment logs")
	s.Contains(output, "--triggerer")
}

func (s *Suite) TestDeploymentLogsRootCommandTriggererDisabled() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		TriggererEnabled: false,
		Flags: houston.FeatureFlags{
			TriggererEnabled: false,
		},
	}

	output, err := execDeploymentCmd("logs", "--help")
	s.NoError(err)
	s.Contains(output, "astro deployment logs")
	s.NotContains(output, "triggerer")
}

func (s *Suite) TestDeploymentLogsWebServerRemoteLogs() {
	for _, test := range []struct {
		component string
		args      []string
	}{
		{component: "webserver", args: []string{"logs", "webserver", mockDeployment.ID}},
		{component: "scheduler", args: []string{"logs", "scheduler", mockDeployment.ID}},
		{component: "workers", args: []string{"logs", "workers", mockDeployment.ID}},
		{component: "triggerer", args: []string{"logs", mockDeployment.ID, "--triggerer"}},
	} {
		mockLogs := getTestLogs(test.component)
		appConfig = &houston.AppConfig{
			TriggererEnabled: true,
			Flags: houston.FeatureFlags{
				TriggererEnabled: true,
			},
		}

		s.Run(fmt.Sprintf("list %s logs success", test.component), func() {
			api := new(mocks.ClientInterface)
			// Have to use mock.Anything because since is computed in the function by using time.Now()
			api.On("ListDeploymentLogs", mock.Anything).Return(mockLogs, nil)
			api.On("GetPlatformVersion", nil).Return("0.25.0", nil)

			houstonClient = api
			output, err := execDeploymentCmd(test.args...)
			s.NoError(err)
			s.Contains(output, mockLogs[0].Log)
			s.Contains(output, mockLogs[1].Log)
		})

		s.Run(fmt.Sprintf("list %s logs error", test.component), func() {
			api := new(mocks.ClientInterface)
			api.On("ListDeploymentLogs", mock.Anything).Return(nil, errMock)
			api.On("GetPlatformVersion", nil).Return("0.25.0", nil)

			houstonClient = api
			_, err := execDeploymentCmd(test.args...)
			s.ErrorIs(errMock, err)
		})
	}
}
