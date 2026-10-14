package deployment

import (
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCheckHibernation() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	d := &houston.Deployment{ID: "dep-id", ClusterID: "cluster-id", Workspace: houston.Workspace{ID: "ws-id"}}
	scoped := houston.GetAppConfigRequest{ClusterID: "cluster-id", WorkspaceUUID: "ws-id", DeploymentUUID: "dep-id"}
	on, off := true, false

	for _, c := range []struct {
		name string
		cfg  *houston.AppConfig
		want error
	}{
		{"enabled", &houston.AppConfig{Version: "2.2.1", Flags: houston.FeatureFlags{HibernationEnabled: &on}}, nil},
		{"disabled", &houston.AppConfig{Version: "2.2.1", Flags: houston.FeatureFlags{HibernationEnabled: &off}}, errHibernationDisabled},
		// 2.2.0 takes an override but does not report the flag: Houston
		// decides.
		{"not reported", &houston.AppConfig{Version: "2.2.0"}, nil},
		{"older platform", &houston.AppConfig{Version: "2.1.27"}, errHibernationNotServed},
	} {
		s.Run(c.name, func() {
			api := new(mocks.ClientInterface)
			api.On("GetDeployment", "dep-id").Return(d, nil).Once()
			// The settings asked about are the Deployment's own, not the
			// platform's.
			api.On("GetAppConfig", scoped).Return(c.cfg, nil).Once()

			err := CheckHibernation("dep-id", api)
			if c.want == nil {
				s.NoError(err)
			} else {
				s.ErrorIs(err, c.want)
			}
			api.AssertExpectations(s.T())
		})
	}

	s.Run("a Deployment that cannot be read", func() {
		api := new(mocks.ClientInterface)
		api.On("GetDeployment", "dep-id").Return(nil, errGetDeploymentMock).Once()

		s.ErrorIs(CheckHibernation("dep-id", api), errGetDeploymentMock)
		api.AssertNotCalled(s.T(), "GetAppConfig", mock.Anything)
	})
}

func (s *Suite) TestSetHibernationOverride() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	until := time.Date(2099, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	set := &houston.Deployment{ID: "dep-id", ReleaseName: "burning-terrestrial-5940", HibernationOverride: &houston.HibernationOverride{IsHibernating: true, OverrideUntil: &until}}

	s.Run("until a time, sent in UTC", func() {
		api := new(mocks.ClientInterface)
		api.On("HibernateOverrideDeployment", map[string]interface{}{
			"deploymentId":        "dep-id",
			"hibernationOverride": map[string]interface{}{"hibernate": true, "overrideUntil": "2099-01-02T02:04:05Z"},
		}).Return(set, nil).Once()

		d, err := SetHibernationOverride("dep-id", true, &until, api)
		s.NoError(err)
		s.Equal(set, d)
		api.AssertExpectations(s.T())
	})

	s.Run("until removed sends no overrideUntil", func() {
		api := new(mocks.ClientInterface)
		api.On("HibernateOverrideDeployment", map[string]interface{}{
			"deploymentId":        "dep-id",
			"hibernationOverride": map[string]interface{}{"hibernate": false},
		}).Return(set, nil).Once()

		_, err := SetHibernationOverride("dep-id", false, nil, api)
		s.NoError(err)
		api.AssertExpectations(s.T())
	})

	s.Run("an API error is returned", func() {
		api := new(mocks.ClientInterface)
		api.On("HibernateOverrideDeployment", mock.Anything).Return(nil, errUpdateDeploymentMock).Once()

		_, err := SetHibernationOverride("dep-id", true, nil, api)
		s.ErrorIs(err, errUpdateDeploymentMock)
	})
}

func (s *Suite) TestRemoveHibernationOverride() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	api := new(mocks.ClientInterface)
	// A null override clears it; an absent one would leave it.
	api.On("HibernateOverrideDeployment", mock.MatchedBy(func(vars map[string]interface{}) bool {
		v, present := vars["hibernationOverride"]
		return vars["deploymentId"] == "dep-id" && present && v == nil
	})).Return(&houston.Deployment{ID: "dep-id"}, nil).Once()

	d, err := RemoveHibernationOverride("dep-id", api)
	s.NoError(err)
	s.Nil(d.HibernationOverride)
	api.AssertExpectations(s.T())
}

// Below 2.2.0 the decorator refuses the mutation (houston's
// TestUpgradeMutationsAreGatedToTheHoustonsThatServeThem pins where), and
// the user is told what the platform needs rather than which GraphQL API it
// lacks.
func (s *Suite) TestHibernationOverrideOnAnOlderPlatform() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	api := new(mocks.ClientInterface)
	api.On("HibernateOverrideDeployment", mock.Anything).
		Return(nil, houston.ErrAPINotImplemented{APIName: "HibernateOverrideDeployment"}).Once()

	_, err := RemoveHibernationOverride("dep-id", api)
	s.ErrorIs(err, errHibernationNotServed)
}
