package deployment

import (
	"bytes"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
)

func (s *Suite) TestLog() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", mock.AnythingOfType("houston.ListDeploymentLogsRequest")).Return([]houston.DeploymentLog{{ID: "test-id", Log: "test log"}}, nil)
		out := new(bytes.Buffer)

		err := Log("test-id", "test-component", "test", 0, api, out)
		s.NoError(err)
		s.Contains(out.String(), "test log")
	})

	s.Run("houston error", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", mock.AnythingOfType("houston.ListDeploymentLogsRequest")).Return([]houston.DeploymentLog{}, errMock)
		out := new(bytes.Buffer)

		err := Log("test-id", "test-component", "test", 0, api, out)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestSubscribeDeploymentLog() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("success", func() {
		subscribe = func(jwtToken, url, queryMessage string) error {
			return nil
		}

		err := SubscribeDeploymentLog("test-id", "test-component", "test", 0)
		s.NoError(err)
	})

	s.Run("houston failure", func() {
		subscribe = func(jwtToken, url, queryMessage string) error {
			return errMock
		}

		err := SubscribeDeploymentLog("test-id", "test-component", "test", 0)
		s.ErrorIs(err, errMock)
	})
}
