package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

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
	assert.Equal(t, requiresPython(runtimeversions.FallbackAirflowSeries), p.pythonBound(&v1Project{}))

	empty := func() (string, string, runtimeversions.Source) { return "", "", runtimeversions.SourceCatalog }
	p = pickAirflowVersion("", nil, &v1Project{}, empty)
	assert.Equal(t, runtimeversions.FallbackAirflowSeries, p.version)
	assert.Equal(t, runtimeversions.SourceBuiltIn, p.source)

	// A stated pin reports no source, so a caller can tell it from a default.
	p = pickAirflowVersion("3.1", nil, &v1Project{}, nil)
	assert.Empty(t, p.source)
	assert.Equal(t, ">=3.10", p.pythonBound(&v1Project{}))
}
