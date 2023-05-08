package software

import (
	"errors"
	"fmt"

	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/houston"
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

	output, err := execDeploymentCmd("logs")
	s.NoError(err)
	s.Contains(output, "astro deployment logs")
	s.Contains(output, "triggerer")
}

func (s *Suite) TestDeploymentLogsRootCommandTriggererDisabled() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		TriggererEnabled: false,
		Flags: houston.FeatureFlags{
			TriggererEnabled: false,
		},
	}

	output, err := execDeploymentCmd("logs")
	s.NoError(err)
	s.Contains(output, "astro deployment logs")
	s.NotContains(output, "triggerer")
}

func (s *Suite) TestDeploymentLogsWebServerRemoteLogs() {
	for _, test := range []struct {
		component string
	}{
		{component: "webserver"},
		{component: "scheduler"},
		{component: "workers"},
		{component: "triggerer"},
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

			houstonClient = api
			output, err := execDeploymentCmd("logs", test.component, mockDeployment.ID)
			s.NoError(err)
			s.Contains(output, mockLogs[0].Log)
			s.Contains(output, mockLogs[1].Log)
		})

		s.Run(fmt.Sprintf("list %s logs error", test.component), func() {
			api := new(mocks.ClientInterface)
			api.On("ListDeploymentLogs", mock.Anything).Return(nil, errMock)

			houstonClient = api
			_, err := execDeploymentCmd("logs", test.component, mockDeployment.ID)
			s.ErrorIs(errMock, err)
		})
	}
}
