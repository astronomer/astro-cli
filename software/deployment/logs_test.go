package deployment

import (
	"bytes"
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestLog(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", mock.AnythingOfType("houston.ListDeploymentLogsRequest")).Return([]houston.DeploymentLog{{ID: "test-id", Log: "test log"}}, nil)
		out := new(bytes.Buffer)

		err := Log("test-id", "test-component", "test", 0, api, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "test log")
	})

	t.Run("houston error", func(t *testing.T) {
		mockErr := errors.New("api error")
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", mock.AnythingOfType("houston.ListDeploymentLogsRequest")).Return([]houston.DeploymentLog{}, mockErr)
		out := new(bytes.Buffer)

		err := Log("test-id", "test-component", "test", 0, api, out)
		assert.ErrorIs(t, err, mockErr)
	})
}

func TestSubscribeDeploymentLog(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	t.Run("success", func(t *testing.T) {
		subscribe = func(jwtToken, url, queryMessage string) error {
			return nil
		}

		err := SubscribeDeploymentLog("test-id", "test-component", "test", 0)
		assert.NoError(t, err)
	})

	t.Run("houston failure", func(t *testing.T) {
		mockErr := errors.New("api error")
		subscribe = func(jwtToken, url, queryMessage string) error {
			return mockErr
		}

		err := SubscribeDeploymentLog("test-id", "test-component", "test", 0)
		assert.ErrorIs(t, err, mockErr)
	})
}
