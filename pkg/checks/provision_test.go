package checks

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The requirement set has to install an Airflow, whatever the manifest says.
//
// A manifest need not name one: a docker project declaring its own Dockerfile
// builds the image from that file, and the dependency list stops describing
// it. Installing nothing called Airflow spends a long download to arrive at
// "Airflow is not importable".
func TestProjectRequirementsAlwaysInstallAnAirflow(t *testing.T) {
	for _, tc := range []struct {
		name string
		pin  string
		deps []string
		want string
	}{
		{"deps name it, in the shape the project states", "3.1", []string{"apache-airflow==3.1.*", "pandas"}, "apache-airflow==3.1.*"},
		{"deps omit it entirely", "3.1", []string{"pandas"}, "apache-airflow==3.1.*"},
		{"deps omit it and the pin is exact", "3.1.2", []string{"pandas"}, "apache-airflow==3.1.2"},
		{"deps omit it and the pin is a major", "2", nil, "apache-airflow==2.*"},
		{"a provider is not the distribution", "3.1", []string{"apache-airflow-providers-snowflake==5.1.0"}, "apache-airflow==3.1.*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := projectRequirements(tc.pin, tc.deps)
			assert.Contains(t, got, tc.want)
		})
	}
}

// A core-only project already names its Airflow. A full apache-airflow added
// beside it would be a second Airflow requirement, which the resolve has to
// reconcile with the first.
func TestProjectRequirementsKeepACoreOnlyProject(t *testing.T) {
	got := projectRequirements("3.3", []string{"apache-airflow-core==3.3.*", "pandas"})
	assert.Equal(t, []string{"apache-airflow-core==3.3.*", "pandas"}, got)
}

// The dependency as the manifest wrote it is left alone: rebuilding it from
// the pin would ask for "apache-airflow==3.1", which is not a release.
func TestProjectRequirementsDoNotRewriteTheProjectsOwnPin(t *testing.T) {
	got := projectRequirements("3.1", []string{"apache-airflow==3.0.5"})
	assert.Contains(t, got, "apache-airflow==3.0.5")
	assert.NotContains(t, got, "apache-airflow==3.1.*", "the manifest's own pin is the one to install")
}

// Sorted, because the provisioner's cache key hashes this slice in order.
// Without it, reordering two lines in pyproject.toml with no semantic change
// misses the cache and reinstalls Airflow.
func TestProjectRequirementsAreSortedForAStableCacheKey(t *testing.T) {
	a := projectRequirements("3.1", []string{"pandas", "apache-airflow==3.1.*", "requests"})
	b := projectRequirements("3.1", []string{"requests", "apache-airflow==3.1.*", "pandas"})
	assert.Equal(t, a, b, "the same dependencies in a different order must produce the same set")
	assert.True(t, slices.IsSorted(a), "not sorted: %v", a)
}

// A consumer with nowhere to stream notes passes nil, the way Preflight
// allows: Astro Desktop has no text renderer. The production provisioner calls
// progress unconditionally, so an unguarded nil is a panic in a sub-module
// other tools build against.
func TestRunProvisionedAcceptsANilProgress(t *testing.T) {
	prov := &fakeProvisioner{python: "/tmp/py"}
	parser := &fakeTargetParser{report: ParseReport{Dags: []ReportDag{{DagID: "a", File: "dags/a.py"}}}}

	res, err := RunProvisioned(context.Background(), Options{ProjectPath: "/p"},
		ProvisionInput{ProjectPath: "/p", Pin: "3.1", Deps: []string{"apache-airflow==3.1.*"}},
		prov, parser, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, res.DagCount)
	assert.Equal(t, "/tmp/py", parser.gotPython, "the parse should use the interpreter that was built")
}

// A provisioning failure is the caller's to classify; it comes back as it is.
func TestRunProvisionedReturnsTheProvisionerError(t *testing.T) {
	boom := errors.New("no uv")
	_, err := RunProvisioned(context.Background(), Options{ProjectPath: "/p"},
		ProvisionInput{ProjectPath: "/p", Pin: "3.1"},
		&fakeProvisioner{ensureErr: boom}, &fakeTargetParser{}, nil)
	assert.ErrorIs(t, err, boom)
}

// The scratch venv sits outside the project, where uv cannot read the
// project's [tool.uv], so the constraints travel in the spec. Sorted, for the
// same cache-key reason the requirements are.
func TestRunProvisionedCarriesTheProjectsConstraints(t *testing.T) {
	prov := &fakeProvisioner{python: "/tmp/py"}
	constraints := []string{"sqlalchemy<2.1", "pandas<3"}
	_, err := RunProvisioned(context.Background(), Options{ProjectPath: "/p"},
		ProvisionInput{ProjectPath: "/p", Pin: "3.1", Constraints: constraints},
		prov, &fakeTargetParser{}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"pandas<3", "sqlalchemy<2.1"}, prov.gotSpec.Constraints)
	assert.Equal(t, []string{"sqlalchemy<2.1", "pandas<3"}, constraints, "the caller's slice must not be reordered")
}
