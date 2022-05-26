package software

import (
	"errors"
	"fmt"
	"testing"

	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
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

func TestDeploymentLogsRootCommandTriggererEnabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	appConfig = &houston.AppConfig{
		TriggererEnabled: true,
		Flags: houston.FeatureFlags{
			TriggererEnabled: true,
		},
	}

	output, err := execDeploymentCmd("logs")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment logs")
	assert.Contains(t, output, "triggerer")
}

func TestDeploymentLogsRootCommandTriggererDisabled(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	appConfig = &houston.AppConfig{
		TriggererEnabled: false,
		Flags: houston.FeatureFlags{
			TriggererEnabled: false,
		},
	}

	output, err := execDeploymentCmd("logs")
	assert.NoError(t, err)
	assert.Contains(t, output, "astro deployment logs")
	assert.NotContains(t, output, "triggerer")
}

func TestDeploymentLogsWebServerRemoteLogs(t *testing.T) {
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

		t.Run(fmt.Sprintf("list %s logs success", test.component), func(t *testing.T) {
			api := new(mocks.ClientInterface)
			// Have to use mock.Anything because since is computed in the function by using time.Now()
			api.On("ListDeploymentLogs", mock.Anything).Return(mockLogs, nil)

			houstonClient = api
			output, err := execDeploymentCmd("logs", test.component, mockDeployment.ID)
			assert.NoError(t, err)
			assert.Contains(t, output, mockLogs[0].Log)
			assert.Contains(t, output, mockLogs[1].Log)
		})

		t.Run(fmt.Sprintf("list %s logs error", test.component), func(t *testing.T) {
			api := new(mocks.ClientInterface)
			api.On("ListDeploymentLogs", mock.Anything).Return(nil, errMock)

			houstonClient = api
			_, err := execDeploymentCmd("logs", test.component, mockDeployment.ID)
			assert.ErrorIs(t, errMock, err)
		})
	}
}
