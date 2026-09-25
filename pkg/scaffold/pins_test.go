package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

func TestAirflowRequirement(t *testing.T) {
	cases := map[string]string{
		"3":     "apache-airflow==3.*",
		"3.1":   "apache-airflow==3.1.*",
		"2.9":   "apache-airflow==2.9.*",
		"3.1.2": "apache-airflow==3.1.2",
	}
	for version, want := range cases {
		assert.Equal(t, want, airflowRequirement(version), version)
	}
}

func TestPinFromSpec(t *testing.T) {
	pinned := map[string]string{
		"apache-airflow==2.9.3":                        "2.9.3",
		"apache-airflow==2.9":                          "2.9",
		"apache-airflow==3":                            "3",
		"apache-airflow[celery]==2.9.3":                "2.9.3",
		"apache_airflow==2.9.3":                        "2.9.3",
		"APACHE-AIRFLOW==2.9.3":                        "2.9.3",
		"apache-airflow ==2.9.3":                       "2.9.3",
		`apache-airflow==2.9.3; python_version<"3.12"`: "2.9.3",
		"apache-airflow[celery,statsd]==2.9.3 ; sys_platform == 'linux'": "2.9.3",
		// A series, which is what tool.astro.airflow means by "2.9", and the
		// shape airflowRequirement writes. See TestPinReadsWhatItWrites.
		"apache-airflow==2.9.*":                           "2.9",
		"apache-airflow==3.*":                             "3",
		"apache-airflow[celery]==3.1.*":                   "3.1",
		"apache-airflow==3.1.* ; sys_platform == 'linux'": "3.1",
	}
	for spec, want := range pinned {
		got, ok := pinFromSpec(spec)
		assert.True(t, ok, spec)
		assert.Equal(t, want, got, spec)
	}

	// A version this cannot read exactly yields no pin, and the caller falls
	// back to the default rather than guessing which release is meant.
	unpinned := []string{
		"apache-airflow>=2.9,<3",
		"apache-airflow~=2.9.3",
		"apache-airflow==2.*.3",
		// Another specifier BEFORE the "==". strings.Cut keeps only what
		// follows the first one, so these reach the version check as a clean
		// "2.9.*" with the rest discarded — refused until now only because the
		// star check happened to catch the wildcard, which reading the series
		// stopped doing.
		"apache-airflow>=2.9,==2.9.*",
		"apache-airflow<3,==2.10.*",
		"apache-airflow!=2.9.1,==2.9.*",
		"apache-airflow[celery]>=2.9,==2.9.*",
		"apache-airflow",
		"apache-airflow==2.9.3,!=2.9.4",
		"apache-airflow @ https://example.com/airflow.whl",
		"apache-airflow-providers-snowflake==5.1.0",
		"pandas==2.0.0",
		"",
	}
	for _, spec := range unpinned {
		_, ok := pinFromSpec(spec)
		assert.False(t, ok, spec)
	}
}

// Every pin this package writes, it must be able to read back.
//
// It could not, and the gap was the shape it writes most: airflowRequirement
// turns "3.1" into "apache-airflow==3.1.*", and pinFromSpec refused anything
// with a star. So adopting a project pinned the way a scaffolded project is
// pinned fell through to the next source in the chain — and where that source
// was a v1 Dockerfile, the result was a manifest whose [tool.astro] pin its own
// dependency contradicted.
func TestPinReadsWhatItWrites(t *testing.T) {
	for _, version := range []string{"2", "2.9", "2.10", "3", "3.1", "3.1.2"} {
		spec := airflowRequirement(version)
		got, ok := pinFromSpec(spec)
		assert.True(t, ok, "%s -> %s could not be read back", version, spec)
		assert.Equal(t, version, got, "%s -> %s", version, spec)
	}
}

func TestNamesAirflow(t *testing.T) {
	for _, spec := range []string{
		"apache-airflow",
		"apache-airflow==2.9.3",
		"apache_airflow>=2.9",
		"apache-airflow[celery]==2.9.3",
		"APACHE-AIRFLOW==2.9.3",
	} {
		assert.True(t, namesAirflow(spec), spec)
	}
	// The providers are their own distributions, and a name that merely
	// contains "airflow" is not the core one.
	for _, spec := range []string{
		"apache-airflow-providers-snowflake==5.1.0",
		"apache-airflow-task-sdk",
		"airflow-exporter",
		"pandas",
	} {
		assert.False(t, namesAirflow(spec), spec)
	}
}

// The precedence chain, which is the one place the ordering is written down.
//
// Two of these orderings were wrong before review, and each produced a project
// pinned a whole Airflow generation from where it actually was. They are
// asserted here rather than in the arms because both arms call this, and the
// bugs were that they disagreed.
func TestPickAirflowVersion(t *testing.T) {
	dockerfile := &v1Project{airflow: "3.1"}

	// The default is the last rung, and resolved only when reached: a project
	// that states its Airflow must not cause a catalog request.
	lookups := 0
	resolve := func() (series, requiresPython string, src runtimeversions.Source) {
		lookups++
		return "3.50", ">=3.13", runtimeversions.SourceCatalog
	}
	pick := func(flag string, deps []string, v1 *v1Project) (string, bool) {
		p := pickAirflowVersion(flag, deps, v1, resolve)
		return p.version, p.defaulted()
	}

	// The caller's option wins over everything. This is what lets Plan stay
	// offline: an Airflow 2 tag names no minor, so a caller that wants the exact
	// one resolves it and passes it here.
	version, defaulted := pick("2.10", []string{"apache-airflow==2.9.3"}, dockerfile)
	assert.Equal(t, "2.10", version)
	assert.False(t, defaulted)

	// Then the MANIFEST's own pin, above the Dockerfile. Folding the Dockerfile
	// into the flag slot put it on top, so adopting a manifest pinning 2.9.3
	// beside a stale runtime:3.1-12 Dockerfile wrote airflow = "3.1" next to a
	// dependency list still saying 2.9.3 — a manifest contradicting itself, with
	// the image built for one and the venv installing the other.
	version, defaulted = pick("", []string{"pandas", "apache-airflow==2.9.3"}, dockerfile)
	assert.Equal(t, "2.9.3", version, "a pin the manifest's author wrote outranks a Dockerfile tag")
	assert.False(t, defaulted)

	// The same, pinned as a series. This is the shape a scaffolded project
	// carries, so it is the shape a real adoption meets most often, and it
	// went to the Dockerfile instead: airflow = "2" written beside
	// apache-airflow==3.0.*.
	version, defaulted = pick("", []string{"apache-airflow==3.0.*"}, &v1Project{airflow: "2"})
	assert.Equal(t, "3.0", version, "a series the manifest pins outranks a Dockerfile tag")
	assert.False(t, defaulted)

	// Then the Dockerfile, above a requirements.txt pin: the image tag is what
	// the project runs today, and a requirements.txt pin is what pip was asked
	// to install into that image.
	version, defaulted = pick("", nil, &v1Project{
		airflow:      "3.1",
		dependencies: []string{"apache-airflow==2.9.3"},
	})
	assert.Equal(t, "3.1", version)
	assert.False(t, defaulted)

	// Then a requirements.txt pin, with no manifest pin and no Dockerfile. The
	// adopt arm used to pass only the manifest's dependencies, so the same
	// project answered differently depending on whether an unrelated
	// pyproject.toml happened to exist: greenfield read this pin, adopt
	// defaulted and then dropped it during the merge.
	version, defaulted = pick("", nil, &v1Project{dependencies: []string{"apache-airflow==2.9.3"}})
	assert.Equal(t, "2.9.3", version, "requirements.txt is read on both arms, not just greenfield")
	assert.False(t, defaulted)

	assert.Zero(t, lookups, "a stated pin asked the resolver")

	// A pin in a shape no version can be read out of is not a pin.
	version, defaulted = pick("", []string{"apache-airflow>=2.9,<3"}, &v1Project{})
	assert.Equal(t, "3.50", version)
	assert.True(t, defaulted)

	// Nothing stated anything.
	version, defaulted = pick("", nil, &v1Project{})
	assert.Equal(t, "3.50", version)
	assert.True(t, defaulted)
	assert.Equal(t, 2, lookups)
}

// With no resolver, or one that names nothing, the default is the built-in
// series, and its requires-python the built-in rule's.
func TestPickAirflowVersionWithoutACatalog(t *testing.T) {
	p := pickAirflowVersion("", nil, &v1Project{}, nil)
	assert.Equal(t, runtimeversions.FallbackAirflowSeries, p.version)
	assert.Equal(t, runtimeversions.SourceBuiltIn, p.source)
	assert.Equal(t, requiresPython(runtimeversions.FallbackAirflowSeries), p.pythonBound())

	empty := func() (string, string, runtimeversions.Source) { return "", "", runtimeversions.SourceCatalog }
	p = pickAirflowVersion("", nil, &v1Project{}, empty)
	assert.Equal(t, runtimeversions.FallbackAirflowSeries, p.version)
	assert.Equal(t, runtimeversions.SourceBuiltIn, p.source)

	// A stated pin reports no source, so a caller can tell it from a default.
	p = pickAirflowVersion("3.1", nil, &v1Project{}, nil)
	assert.Empty(t, p.source)
	assert.Equal(t, ">=3.10", p.pythonBound())
}
