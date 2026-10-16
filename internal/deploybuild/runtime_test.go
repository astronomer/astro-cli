package deploybuild

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

func TestCheckRuntimeVersion(t *testing.T) {
	// No current version: nothing to check.
	require.NoError(t, checkRuntimeVersion("", "7.0.0", nil, nil))
	// Supported and equal: fine.
	require.NoError(t, checkRuntimeVersion("7.0.0", "7.0.0", []string{"7.0.0"}, nil))
	// Downgrade.
	assert.ErrorContains(t, checkRuntimeVersion("7.0.0", "6.0.0", []string{"6.0.0", "7.0.0"}, nil), "downgrade")
	// Not in the allowed set.
	assert.ErrorContains(t, checkRuntimeVersion("7.0.0", "8.0.0", []string{"7.0.0"}, nil), "unsupported")
	// A downgrade names the fix when the caller knows it.
	err := checkRuntimeVersion("3.3-8", "3.3-7", []string{"3.3-7", "3.3-8"}, func(minimum string) string { return "use " + minimum })
	assert.EqualError(t, err, "cannot deploy Astro Runtime 3.3-7: it is a downgrade from the deployment's current 3.3-8; to deploy, use 3.3-8")
}

// The rules a built image's runtime is held to, as plain inputs: the
// Deployment's current runtime, the image's, and the runtimes the Deployment
// accepts. Whoever lists the accepted runtimes (Astro's deployment options, or
// another control plane's catalog) gets the same verdicts.
func TestRuntimeRules(t *testing.T) {
	accepted := []string{"12.1.0", "12.2.0", "3.1-1", "3.1-2", "3.2-1"}
	tests := []struct {
		name    string
		current string
		built   string
		wantErr string
	}{
		{name: "same version", current: "3.1-2", built: "3.1-2"},
		{name: "a newer build", current: "3.1-1", built: "3.1-2"},
		{name: "a newer series", current: "3.1-2", built: "3.2-1"},
		{name: "a downgrade", current: "3.1-2", built: "3.1-1", wantErr: "cannot deploy Astro Runtime 3.1-1: it is a downgrade from the deployment's current 3.1-2; to deploy, raise to 3.1-2"},
		{name: "an Airflow 2 downgrade", current: "12.2.0", built: "12.1.0", wantErr: "downgrade from the deployment's current 12.2.0"},
		{name: "newer than the newest accepted", current: "3.2-1", built: "3.3-1", wantErr: "cannot deploy unsupported Astro Runtime 3.3-1; supported versions: 12.1.0, 12.2.0, 3.1-1, 3.1-2, 3.2-1"},
		{name: "unlisted between listed ones", current: "3.1-1", built: "3.1-3", wantErr: "cannot deploy unsupported Astro Runtime 3.1-3"},
		{name: "Airflow 3 to 2 is a downgrade", current: "3.1-2", built: "12.2.0", wantErr: "cannot deploy Astro Runtime 12.2.0: it is a downgrade from the deployment's current 3.1-2"},
		{name: "Airflow 2 to 3 from 12.0.0 up", current: "12.1.0", built: "3.1-2"},
		{name: "Airflow 2 to 3 below 12.0.0", current: "11.9.0", built: "3.1-2", wantErr: "cannot upgrade from Airflow 2 to Airflow 3 unless the deployment is at Astro Runtime 12.0.0 or higher (currently 11.9.0)"},
		{name: "no current version", current: "", built: "9.9-9"},
	}
	p := Plan{raise: func(minimum string) string { return "raise to " + minimum }}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.CheckBuilt(tt.current, tt.built, accepted)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestPlanCheck(t *testing.T) {
	offered := []string{"3.2-10", "3.3-7", "3.3-8", "3.10-1"}
	raise := func(minimum string) string { return "raise to " + minimum }
	tests := []struct {
		name    string
		current string
		planned Plan
		wantErr string
	}{
		{name: "nothing planned", current: "3.3-8", planned: Plan{}},
		{name: "no current version", current: "", planned: Plan{version: "3.2", series: true}},
		{name: "older series", current: "3.3-8", planned: Plan{version: "3.2", series: true, raise: raise}, wantErr: "cannot deploy Astro Runtime 3.2: it is a downgrade from the deployment's current 3.3-8; to deploy, raise to 3.3-8"},
		{name: "same series leaves the patch to the label", current: "3.3-8", planned: Plan{version: "3.3", series: true}},
		{name: "series not offered", current: "3.3-8", planned: Plan{version: "3.5", series: true}, wantErr: "unsupported"},
		{name: "series compared as numbers", current: "3.9-1", planned: Plan{version: "3.10", series: true}},
		{name: "series over an Airflow 2 deployment below the floor", current: "11.0.0", planned: Plan{version: "3.3", series: true}, wantErr: "Airflow 2 to Airflow 3"},
		{name: "series over an Airflow 2 deployment at the floor", current: "12.1.0", planned: Plan{version: "3.3", series: true}},
		{name: "exact downgrade", current: "3.3-8", planned: Plan{version: "3.3-7", raise: raise}, wantErr: "downgrade from the deployment's current 3.3-8; to deploy, raise to 3.3-8"},
		{name: "exact not offered", current: "3.3-8", planned: Plan{version: "3.3-9"}, wantErr: "unsupported"},
		{name: "exact fine", current: "3.3-7", planned: Plan{version: "3.3-8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.planned.Check(tt.current, offered)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestPlanRuntimeReadsTheGeneratedBase(t *testing.T) {
	series := PlanRuntime("", &imagebuild.Request{BaseImage: imagebuild.RuntimeImageRepo + ":3.3"}, &imagebuild.ManifestBuild{})
	assert.Equal(t, "3.3", series.version)
	assert.True(t, series.series)

	exact := PlanRuntime("", &imagebuild.Request{BaseImage: imagebuild.RuntimeImageRepo + ":3.3-8-python-3.13"}, &imagebuild.ManifestBuild{})
	assert.Equal(t, "3.3-8", exact.version)
	assert.False(t, exact.series)
	assert.Equal(t, "set [tool.astro] runtime to 3.3-9 or newer in pyproject.toml", exact.raise("3.3-9"))
}

// Each source of the image names its own fix for a downgrade.
func TestPlanRuntimeNamesTheFix(t *testing.T) {
	prebuilt := PlanRuntime("astro-package/demo:3.2-10", &imagebuild.Request{}, &imagebuild.ManifestBuild{})
	assert.Empty(t, prebuilt.version, "a prebuilt image is known only by its label")
	assert.Equal(t, "rebuild astro-package/demo:3.2-10 FROM Astro Runtime 3.3-8 or newer", prebuilt.raise("3.3-8"))

	runtime := PlanRuntime("", &imagebuild.Request{}, &imagebuild.ManifestBuild{Runtime: "3.2-10"})
	assert.Equal(t, "3.2-10", runtime.version)
	assert.False(t, runtime.series)
	assert.Equal(t, "pin apache-airflow to 3.3 and set [tool.astro] runtime to 3.3-8 or newer in pyproject.toml", runtime.raise("3.3-8"))
}
