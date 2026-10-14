package apc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

var hibernatingDep = houston.Deployment{ID: "dep-hb", Label: "alpha", ReleaseName: "rel-hb", ClusterID: "cl-1", Workspace: houston.Workspace{ID: "ws-1"}}

// newHibernationClient is newAPCClient on a platform that serves hibernation
// with it enabled for dep-hb, whose hibernation override leaves it as left.
func newHibernationClient(left *houston.Deployment) *mocks.ClientInterface {
	on := true
	api := newAPCClient()
	api.On("GetDeployment", "dep-hb").Return(&hibernatingDep, nil).Maybe()
	// The settings asked about are the Deployment's own; any other lookup
	// is the tree's, answered as newAPCClient answers it.
	api.On("GetAppConfig", mock.Anything).Unset()
	api.On("GetAppConfig", houston.GetAppConfigRequest{ClusterID: "cl-1", WorkspaceUUID: "ws-1", DeploymentUUID: "dep-hb"}).
		Return(&houston.AppConfig{Version: "2.2.1", Flags: houston.FeatureFlags{HibernationEnabled: &on}}, nil).Maybe()
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Flags: houston.FeatureFlags{AstroRuntimeEnabled: true, TriggererEnabled: true}}, nil).Maybe()
	if left != nil {
		api.On("HibernateOverrideDeployment", mock.Anything).Return(left, nil).Maybe()
	}
	return api
}

func withOverride(hibernate bool, until *time.Time) *houston.Deployment {
	d := hibernatingDep
	d.HibernationOverride = &houston.HibernationOverride{IsHibernating: hibernate, OverrideUntil: until}
	return &d
}

func TestDeploymentHibernationJSON(t *testing.T) {
	until := time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("hibernate --for publishes the override it set", func(t *testing.T) {
		api := newHibernationClient(withOverride(true, &until))

		run := runAPC(t, api, "", "deployment", "hibernate", "dep-hb", "--for", "2h", "--yes", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentHibernationJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deploymentHibernationJSON{
			DeploymentID: "dep-hb", Label: "alpha", ReleaseName: "rel-hb",
			HibernationOverride: &hibernationOverrideJSON{IsHibernating: true, OverrideUntil: &until},
		}, got)
		api.AssertCalled(t, "HibernateOverrideDeployment", mock.MatchedBy(func(vars map[string]interface{}) bool {
			o, ok := vars["hibernationOverride"].(map[string]interface{})
			return ok && vars["deploymentId"] == "dep-hb" && o["hibernate"] == true && o["overrideUntil"] != nil
		}))
	})

	t.Run("wake-up --remove-override publishes no override", func(t *testing.T) {
		api := newHibernationClient(&hibernatingDep)

		run := runAPC(t, api, "", "deployment", "wake-up", "dep-hb", "--remove-override", "--yes", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentHibernationJSON
		decodeOne(t, run.stdout, &got)
		assert.Nil(t, got.HibernationOverride)
		api.AssertCalled(t, "HibernateOverrideDeployment", mock.MatchedBy(func(vars map[string]interface{}) bool {
			v, present := vars["hibernationOverride"]
			return present && v == nil
		}))
	})

	t.Run("the question is refused under json", func(t *testing.T) {
		api := newHibernationClient(nil)

		run := runAPC(t, api, "y\n", "deployment", "hibernate", "dep-hb", "-o", "json")
		assert.Equal(t, 1, run.code)
		var got errorObject
		decodeOne(t, run.stdout, &got)
		assert.Contains(t, got.Error, "--yes")
		assert.False(t, run.stdinRead, "a refused prompt read stdin")
		api.AssertNotCalled(t, "HibernateOverrideDeployment", mock.Anything)
	})
}

func TestDeploymentHibernationText(t *testing.T) {
	until := time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, c := range []struct {
		name string
		args []string
		left *houston.Deployment
		want string
	}{
		{"hibernate until removed", []string{"hibernate", "dep-hb"}, withOverride(true, nil), "\nSuccessfully set deployment rel-hb to hibernate until you remove the override.\n"},
		{"wake-up until a time", []string{"wake-up", "dep-hb", "--until", "2099-01-02T03:04:05Z"}, withOverride(false, &until), "\nSuccessfully woke up deployment rel-hb until 2099-01-02T03:04:05Z.\n"},
		{"remove the override", []string{"hibernate", "dep-hb", "--remove-override"}, &hibernatingDep, "\nSuccessfully removed the hibernation override on deployment rel-hb. Its hibernation schedule, if it has one, resumes now.\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			api := newHibernationClient(c.left)

			run := runAPC(t, api, "", append(append([]string{"deployment"}, c.args...), "--yes")...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Equal(t, c.want, run.stdout)
		})
	}

	t.Run("a declined question changes nothing", func(t *testing.T) {
		api := newHibernationClient(nil)

		run := runAPC(t, api, "n\n", "deployment", "wake-up", "dep-hb")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Contains(t, run.stdout, "your Deployment was not changed")
		api.AssertNotCalled(t, "HibernateOverrideDeployment", mock.Anything)
	})
}

func TestDeploymentHibernationRefusals(t *testing.T) {
	t.Run("hibernation disabled for the Deployment", func(t *testing.T) {
		off := false
		api := newAPCClient()
		api.On("GetDeployment", "dep-hb").Return(&hibernatingDep, nil)
		api.On("GetAppConfig", mock.Anything).Unset()
		api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "2.2.1", Flags: houston.FeatureFlags{HibernationEnabled: &off}}, nil)

		run := runAPC(t, api, "", "deployment", "hibernate", "dep-hb", "--yes")
		assert.Equal(t, 1, run.code)
		assert.Contains(t, run.err.Error(), "hibernation is not enabled for this Deployment")
		api.AssertNotCalled(t, "HibernateOverrideDeployment", mock.Anything)
	})

	t.Run("a platform before 2.2.0", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-hb").Return(&hibernatingDep, nil)
		api.On("GetAppConfig", mock.Anything).Unset()
		api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "2.1.27"}, nil)

		run := runAPC(t, api, "", "deployment", "wake-up", "dep-hb", "--yes")
		assert.Equal(t, 1, run.code)
		assert.Contains(t, run.err.Error(), "needs Astro Private Cloud 2.2.0 or later")
		api.AssertNotCalled(t, "HibernateOverrideDeployment", mock.Anything)
	})

	for _, c := range []struct {
		name string
		args []string
	}{
		{"--until and --for together", []string{"--until", "2099-01-02T03:04:05Z", "--for", "2h"}},
		{"--for and --remove-override together", []string{"--for", "2h", "--remove-override"}},
		{"a --for that is no duration", []string{"--for", "soon"}},
		{"an --until that is no time", []string{"--until", "tomorrow"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			api := newHibernationClient(nil)

			run := runAPC(t, api, "", append([]string{"deployment", "hibernate", "dep-hb", "--yes"}, c.args...)...)
			assert.NotEqual(t, 0, run.code)
			api.AssertNotCalled(t, "HibernateOverrideDeployment", mock.Anything)
		})
	}

	// --force was the 1.x spelling of --yes; the tombstone names --yes.
	t.Run("--force", func(t *testing.T) {
		api := newHibernationClient(nil)

		run := runAPC(t, api, "", "deployment", "hibernate", "dep-hb", "--force")
		assert.NotEqual(t, 0, run.code)
		api.AssertNotCalled(t, "HibernateOverrideDeployment", mock.Anything)
	})
}
