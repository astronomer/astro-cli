package imagebuild

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// fullManifest sets everything ManifestBuild reads. A literal rather than a
// parsed file, because Parse refuses a runtime build beside a Dockerfile, and
// this needs both set at once.
func fullManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Project: manifest.Project{
			Name:           "p",
			RequiresPython: "==3.13.*",
			Dependencies:   []string{"apache-airflow==3.1.*", "pandas", "private-lib"},
		},
		Astro: manifest.Astro{
			Runtime:    "3.1-2",
			Dockerfile: "docker/Dockerfile",
			Packages:   []string{"libpq-dev"},
		},
		UV: manifest.UV{
			Sources: map[string]string{"private-lib": "https://pypi.acme.example/simple/"},
		},
	}
}

// TestManifestBuildOfFillsEveryField keeps the manifest-to-image mapping in
// one place. Every consumer (local start, deploy, `astro package astro`,
// Astro Desktop) fills a ManifestBuild through ManifestBuildOf, so a field
// added to ManifestBuild that this function does not set would reach all of
// them as its zero value.
func TestManifestBuildOfFillsEveryField(t *testing.T) {
	v := reflect.ValueOf(ManifestBuildOf("/project", fullManifest()))
	for i := range v.NumField() {
		if v.Field(i).IsZero() {
			t.Errorf("ManifestBuildOf leaves ManifestBuild.%s zero for a manifest that sets everything: "+
				"wire the new field in ManifestBuildOf (pkg/imagebuild/manifest.go), and set it in fullManifest",
				v.Type().Field(i).Name)
		}
	}
}

func TestManifestBuildOf(t *testing.T) {
	assert.Equal(t, ManifestBuild{
		ProjectDir:     "/project",
		AirflowVersion: "3.1",
		Runtime:        "3.1-2",
		RequiresPython: "==3.13.*",
		Dockerfile:     "docker/Dockerfile",
		Dependencies:   []string{"apache-airflow==3.1.*", "pandas", "private-lib"},
		FindLinks:      []string{"https://pypi.acme.example/simple/private-lib/"},
		Packages:       []string{"libpq-dev"},
	}, ManifestBuildOf("/project", fullManifest()))

	assert.Equal(t, ManifestBuild{ProjectDir: "/project"}, ManifestBuildOf("/project", nil))
}

// The [tool.uv] settings `astro init` writes route Airflow, its core and its
// Task SDK to Astronomer's index. A generated image installs none of them
// (runtimeDeps), so none of their pages reaches the build: an init'd
// project's build reads only PyPI, as it did before index pages were passed,
// and a host that cannot reach pip.astronomer.io still builds.
func TestManifestBuildOfLeavesOutTheAirflowSourcesInitWrites(t *testing.T) {
	m := &manifest.Manifest{
		Project: manifest.Project{
			Name:         "p",
			Dependencies: []string{"apache-airflow==3.3.*", "apache-airflow-core", "apache-airflow-task-sdk", "pandas"},
		},
		UV: manifest.UV{Sources: map[string]string{
			"apache-airflow":          "https://pip.astronomer.io/v2/",
			"apache-airflow-core":     "https://pip.astronomer.io/v2/",
			"apache-airflow-task-sdk": "https://pip.astronomer.io/v2/",
		}},
	}
	mb := ManifestBuildOf("/project", m)
	assert.Empty(t, mb.FindLinks)

	req, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.3", Dependencies: mb.Dependencies, FindLinks: mb.FindLinks}, nil)
	assert.NoError(t, err)
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-deploy/p", "docker"
	_, err = testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	assert.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, requirementsName))
	assert.NoError(t, err)
	assert.Equal(t, "pandas\n", string(got), "no --find-links line")
}
