package imagebuild

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// fullManifest sets everything ManifestBuild reads. A literal rather than a
// parsed file, because Parse refuses a runtime build beside a Dockerfile, and
// this needs both set at once.
func fullManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Project: manifest.Project{
			Name:         "p",
			Dependencies: []string{"apache-airflow==3.1.*", "pandas"},
		},
		Astro: manifest.Astro{
			Runtime:    "3.1-2",
			Dockerfile: "docker/Dockerfile",
			Packages:   []string{"libpq-dev"},
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
		Dockerfile:     "docker/Dockerfile",
		Dependencies:   []string{"apache-airflow==3.1.*", "pandas"},
		Packages:       []string{"libpq-dev"},
	}, ManifestBuildOf("/project", fullManifest()))

	assert.Equal(t, ManifestBuild{ProjectDir: "/project"}, ManifestBuildOf("/project", nil))
}
