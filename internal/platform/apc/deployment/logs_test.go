package deployment

import (
	"io"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestLog() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", mock.MatchedBy(func(r houston.ListDeploymentLogsRequest) bool {
			return r.DeploymentID == "test-id" && r.Component == "test-component" && r.Search == "test"
		})).Return([]houston.DeploymentLog{{ID: "test-id", Log: "test log"}}, nil)

		logs, err := Log("test-id", "test-component", "test", 0, api)
		s.NoError(err)
		s.Equal([]houston.DeploymentLog{{ID: "test-id", Log: "test log"}}, logs)
	})

	// --since is a window: startTime and endTime, which Houston searches
	// exactly; timestamp alone is a whole UTC day there.
	s.Run("since is sent as a window", func() {
		api := new(mocks.ClientInterface)
		var got houston.ListDeploymentLogsRequest
		api.On("ListDeploymentLogs", mock.AnythingOfType("houston.ListDeploymentLogsRequest")).Run(func(args mock.Arguments) {
			got = args.Get(0).(houston.ListDeploymentLogsRequest)
		}).Return([]houston.DeploymentLog{}, nil)

		_, err := Log("test-id", "scheduler", "", 5*time.Minute, api)
		s.NoError(err)
		s.Require().NotNil(got.LogWindow)
		s.InDelta(5*time.Minute, got.EndTime.Sub(got.StartTime), float64(time.Second))

		_, err = Log("test-id", "scheduler", "", 0, api)
		s.NoError(err)
		s.Nil(got.LogWindow, "no --since: today's logs, as before")
	})

	s.Run("houston error", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", mock.AnythingOfType("houston.ListDeploymentLogsRequest")).Return([]houston.DeploymentLog{}, errMock)

		_, err := Log("test-id", "test-component", "test", 0, api)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestSubscribeDeploymentLog() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	prev := subscribe
	s.T().Cleanup(func() { subscribe = prev })

	s.Run("success hands each record on", func() {
		subscribe = func(_, _, _ string, _ io.Writer, onLog func(houston.DeploymentLog) error) error {
			return onLog(houston.DeploymentLog{Log: "a record"})
		}

		var got []string
		err := SubscribeDeploymentLog("test-id", "test-component", "test", 0, io.Discard, func(l houston.DeploymentLog) error {
			got = append(got, l.Log)
			return nil
		})
		s.NoError(err)
		s.Equal([]string{"a record"}, got)
	})

	s.Run("houston failure", func() {
		subscribe = func(_, _, _ string, _ io.Writer, _ func(houston.DeploymentLog) error) error {
			return errMock
		}

		err := SubscribeDeploymentLog("test-id", "test-component", "test", 0, io.Discard, func(houston.DeploymentLog) error { return nil })
		s.ErrorIs(err, errMock)
	})
}
