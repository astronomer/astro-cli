package deploybuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What Astro Private Cloud refuses of a pushed image's runtime, as
// houston-api checks it after the push (validateRuntimeVersionForUpdateImage
// and validateRuntimeVersion), refused before it: a downgrade from the
// Deployment's current runtime, Airflow 3 to 2 included, and only when it has
// one; and a runtime newer than the newest listed. Anything else the list
// does not hold deploys, with a warning: the list is the newest build of each
// Airflow version, and Houston takes any build it knows. Airflow 2 to 3 is an
// upgrade with no floor, which Astro's rules have and Houston's do not.
func TestListedRuntimeRules(t *testing.T) {
	listed := []string{"3.1-5", "3.0-9", "12.1.1", "11.9.0"}
	tests := []struct {
		name     string
		current  string
		built    string
		listed   []string
		wantErr  string
		wantWarn string
	}{
		{name: "same version", current: "3.1-5", built: "3.1-5", listed: listed},
		{name: "a newer listed series", current: "3.0-9", built: "3.1-5", listed: listed},
		{name: "Airflow 2 to 3, from below 12.0.0 too", current: "11.9.0", built: "3.1-5", listed: listed},
		{name: "a downgrade", current: "3.1-5", built: "3.0-9", listed: listed, wantErr: "cannot deploy Astro Runtime 3.0-9: it is a downgrade from the deployment's current 3.1-5; to deploy, raise to 3.1-5"},
		{name: "Airflow 3 to 2 is a downgrade", current: "3.0-9", built: "12.1.1", listed: listed, wantErr: "it is a downgrade from the deployment's current 3.0-9"},
		{name: "newer than the newest listed", current: "3.1-5", built: "3.2-1", listed: listed, wantErr: "cannot deploy Astro Runtime 3.2-1: it is newer than the newest the platform lists for this Deployment, 3.1-5; listed: 3.1-5, 3.0-9, 12.1.1, 11.9.0"},
		{name: "newer than the newest listed, with no current runtime", current: "", built: "3.2-1", listed: listed, wantErr: "newer than the newest"},
		{name: "an older build of a listed Airflow warns", current: "3.0-9", built: "3.1-4", listed: listed, wantWarn: "Astro Runtime 3.1-4 is not one the platform lists for this Deployment (3.1-5, 3.0-9, 12.1.1, 11.9.0)"},
		{name: "unlisted with no current runtime warns", current: "", built: "3.1-4", listed: listed, wantWarn: "is not one the platform lists"},
		{name: "a downgrade with no current runtime is not one", current: "", built: "11.9.0", listed: listed},
		{name: "Airflow 3 where none is listed", current: "12.1.1", built: "3.1-5", listed: []string{"12.1.1", "11.9.0"}, wantErr: "cannot deploy Astro Runtime 3.1-5: Airflow 3 is not enabled for this Deployment (the platform lists only 12.1.1, 11.9.0). Ask the platform admin to enable Airflow 3 for it, or pin Airflow 2 in pyproject.toml"},
		{name: "no list checks the downgrade alone", current: "3.0-9", built: "3.9-1"},
		{name: "no list still refuses a downgrade", current: "3.0-9", built: "12.1.1", wantErr: "downgrade"},
	}
	p := Plan{raise: func(minimum string) string { return "raise to " + minimum }}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning, err := p.CheckBuiltListed(tt.current, tt.built, tt.listed)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantWarn == "" {
				assert.Empty(t, warning)
				return
			}
			assert.Contains(t, warning, tt.wantWarn)
		})
	}
}

// Before the build, an exact runtime takes the refusals as they are, and a
// series refuses only what no build of it can pass. Nothing warns before the
// build: the built label is what is warned of.
func TestListedRuntimeRulesBeforeTheBuild(t *testing.T) {
	listed := []string{"3.1-5", "3.0-9", "12.1.1"}
	exact := Plan{version: "3.0-9"}
	assert.ErrorContains(t, exact.CheckListed("3.1-5", listed), "downgrade")
	require.NoError(t, exact.CheckListed("3.0-8", listed))
	require.NoError(t, (&Plan{version: "3.1-4"}).CheckListed("3.0-9", listed), "unlisted is a warning, on the built label")
	assert.ErrorContains(t, (&Plan{version: "3.2-1"}).CheckListed("3.0-9", listed), "newer than the newest")

	series := Plan{version: "3.1", series: true}
	require.NoError(t, series.CheckListed("3.1-5", listed), "a build of the series can pass")
	require.NoError(t, series.CheckListed("12.1.1", listed), "Airflow 2 to 3")
	assert.ErrorContains(t, (&Plan{version: "3.0", series: true}).CheckListed("3.1-5", listed), "downgrade")
	assert.ErrorContains(t, (&Plan{version: "3.2", series: true}).CheckListed("3.1-5", listed), "newer than the newest the platform lists for this Deployment, 3.1-5")
	require.NoError(t, (&Plan{version: "3.2", series: true}).CheckListed("3.1-5", nil), "nothing listed to hold it to")

	airflow2Only := []string{"12.1.1", "11.9.0"}
	for _, p := range []*Plan{{version: "3.1", series: true}, {version: "3.1-5"}} {
		assert.ErrorContains(t, p.CheckListed("12.1.1", airflow2Only), "Airflow 3 is not enabled for this Deployment", p.version)
	}
	require.NoError(t, (&Plan{version: "12.1.1"}).CheckListed("12.1.1", airflow2Only))

	require.NoError(t, (&Plan{}).CheckListed("3.1-5", listed), "a plan with no version waits for the built image")
}

func TestAirflow3Upgrade(t *testing.T) {
	assert.True(t, Airflow3Upgrade("12.1.1", "3.1-5"))
	assert.True(t, Airflow3Upgrade("11.9.0", "3.0-1"))
	assert.False(t, Airflow3Upgrade("3.0-9", "3.1-5"))
	assert.False(t, Airflow3Upgrade("12.1.0", "12.1.1"))
	assert.False(t, Airflow3Upgrade("", "3.1-5"), "a Deployment with no runtime is not upgraded from Airflow 2")
}
