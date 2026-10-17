package deploybuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rules Astro Private Cloud holds a pushed image's runtime to, as
// houston-api checks them after the push (validateRuntimeVersionForUpdateImage
// and validateRuntimeVersion), held before it: no downgrade, Airflow 3 to 2
// included; nothing newer than the newest listed; one the list holds. Airflow
// 2 to 3 is an upgrade with no floor, which Astro's rules have and Houston's
// do not.
func TestListedRuntimeRules(t *testing.T) {
	listed := []string{"3.1-5", "3.0-9", "12.1.1", "11.9.0"}
	tests := []struct {
		name    string
		current string
		built   string
		listed  []string
		wantErr string
	}{
		{name: "same version", current: "3.1-5", built: "3.1-5", listed: listed},
		{name: "a newer listed series", current: "3.0-9", built: "3.1-5", listed: listed},
		{name: "Airflow 2 to 3, from below 12.0.0 too", current: "11.9.0", built: "3.1-5", listed: listed},
		{name: "a downgrade", current: "3.1-5", built: "3.0-9", listed: listed, wantErr: "cannot deploy Astro Runtime 3.0-9: it is a downgrade from the deployment's current 3.1-5; to deploy, raise to 3.1-5"},
		{name: "Airflow 3 to 2 is a downgrade", current: "3.0-9", built: "12.1.1", listed: listed, wantErr: "it is a downgrade from the deployment's current 3.0-9"},
		{name: "newer than the newest listed", current: "3.1-5", built: "3.2-1", listed: listed, wantErr: "cannot deploy Astro Runtime 3.2-1: it is newer than the newest the platform lists for this Deployment, 3.1-5; listed: 3.1-5, 3.0-9, 12.1.1, 11.9.0"},
		{name: "unlisted", current: "3.0-9", built: "3.1-4", listed: listed, wantErr: "cannot deploy Astro Runtime 3.1-4: the platform does not list it for this Deployment; listed: 3.1-5"},
		{name: "no list checks the downgrade alone", current: "3.0-9", built: "3.9-1"},
		{name: "no list still refuses a downgrade", current: "3.0-9", built: "12.1.1", wantErr: "downgrade"},
		{name: "no current runtime checks the list alone", current: "", built: "3.1-5", listed: listed},
		{name: "no current runtime, unlisted", current: "", built: "3.1-4", listed: listed, wantErr: "does not list it"},
	}
	p := Plan{raise: func(minimum string) string { return "raise to " + minimum }}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.CheckBuiltListed(tt.current, tt.built, tt.listed)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// Before the build, an exact runtime takes the rules as they are, and a
// series refuses only what no build of it can pass.
func TestListedRuntimeRulesBeforeTheBuild(t *testing.T) {
	listed := []string{"3.1-5", "3.0-9", "12.1.1"}
	exact := Plan{version: "3.0-9"}
	assert.ErrorContains(t, exact.CheckListed("3.1-5", listed), "downgrade")
	require.NoError(t, exact.CheckListed("3.0-8", listed))

	series := Plan{version: "3.1", series: true}
	require.NoError(t, series.CheckListed("3.1-5", listed), "a build of the series can pass")
	require.NoError(t, series.CheckListed("12.1.1", listed), "Airflow 2 to 3")
	assert.ErrorContains(t, (&Plan{version: "3.0", series: true}).CheckListed("3.1-5", listed), "downgrade")
	assert.ErrorContains(t, (&Plan{version: "3.2", series: true}).CheckListed("3.1-5", listed), "the platform lists no build of it")
	require.NoError(t, (&Plan{version: "3.2", series: true}).CheckListed("3.1-5", nil), "nothing listed to hold it to")

	require.NoError(t, (&Plan{}).CheckListed("3.1-5", listed), "a plan with no version waits for the built image")
}

func TestAirflow3Upgrade(t *testing.T) {
	assert.True(t, Airflow3Upgrade("12.1.1", "3.1-5"))
	assert.True(t, Airflow3Upgrade("11.9.0", "3.0-1"))
	assert.False(t, Airflow3Upgrade("3.0-9", "3.1-5"))
	assert.False(t, Airflow3Upgrade("12.1.0", "12.1.1"))
	assert.False(t, Airflow3Upgrade("", "3.1-5"), "a Deployment with no runtime is not upgraded from Airflow 2")
}
