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

func (s *Suite) TestLogs() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	of := func(component string) any {
		return mock.MatchedBy(func(r houston.ListDeploymentLogsRequest) bool { return r.Component == component })
	}

	s.Run("one request per component, merged oldest first", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", of("scheduler")).Return([]houston.DeploymentLog{
			{CreatedAt: "2026-01-01T00:00:01Z", Log: "s1"},
			{CreatedAt: "2026-01-01T00:00:03.5Z", Log: "s2"},
		}, nil).Once()
		api.On("ListDeploymentLogs", of("worker")).Return([]houston.DeploymentLog{
			{CreatedAt: "2026-01-01T00:00:02Z", Log: "w1"},
			{CreatedAt: "2026-01-01T00:00:03.25Z", Log: "w2"},
		}, nil).Once()

		logs, err := Logs("dep", []string{"scheduler", "worker"}, "", 0, api)
		s.NoError(err)
		var got []string
		for _, l := range logs {
			got = append(got, l.Component+":"+l.Log)
		}
		s.Equal([]string{"scheduler:s1", "worker:w1", "worker:w2", "scheduler:s2"}, got)
		api.AssertExpectations(s.T())
	})

	s.Run("one component keeps Houston's order, whatever its times", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", of("scheduler")).Return([]houston.DeploymentLog{
			{CreatedAt: "2026-01-01T00:00:02Z", Log: "newer", Component: "scheduler-0"},
			{CreatedAt: "2026-01-01T00:00:01Z", Log: "older"},
			{CreatedAt: "", Log: "untimed"},
		}, nil).Once()

		logs, err := Logs("dep", []string{"scheduler"}, "", 0, api)
		s.NoError(err)
		var got []string
		for _, l := range logs {
			got = append(got, l.Component+":"+l.Log)
		}
		s.Equal([]string{"scheduler:newer", "scheduler:older", "scheduler:untimed"}, got, "the component asked for, in Houston's order")
	})

	s.Run("several with a time it cannot read are left one component after another", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", of("scheduler")).Return([]houston.DeploymentLog{
			{CreatedAt: "2026-01-01T00:00:03Z", Log: "s1"},
		}, nil).Once()
		api.On("ListDeploymentLogs", of("worker")).Return([]houston.DeploymentLog{
			{CreatedAt: "1767225601000", Log: "w1"},
		}, nil).Once()

		logs, err := Logs("dep", []string{"scheduler", "worker"}, "", 0, api)
		s.NoError(err)
		s.Require().Len(logs, 2)
		s.Equal("s1", logs[0].Log)
		s.Equal("w1", logs[1].Log)
	})

	s.Run("a component's failure is the answer", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentLogs", of("scheduler")).Return([]houston.DeploymentLog{{Log: "s1"}}, nil).Once()
		api.On("ListDeploymentLogs", of("worker")).Return(nil, errMock).Once()

		logs, err := Logs("dep", []string{"scheduler", "worker"}, "", 0, api)
		s.ErrorIs(err, errMock)
		s.Nil(logs)
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
