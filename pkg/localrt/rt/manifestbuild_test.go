package rt

// Plain stdlib testing, like its neighbors: this package keeps its dependency
// list empty.

import (
	"reflect"
	"testing"
)

// TestPlanCarriesEveryManifestBuildField guards the copy between a Plan and
// the ManifestBuild an engine asks its ImageBuilder about. A field added to
// ManifestBuild (and Plan) but missed by SetManifestBuild or Plan.ManifestBuild
// would reach docker mode as its zero value, and a project would start from a
// different image than it deploys.
func TestPlanCarriesEveryManifestBuildField(t *testing.T) {
	want := ManifestBuild{
		ProjectDir:     "/project",
		AirflowVersion: "3.1",
		Runtime:        "3.1-2",
		RequiresPython: "==3.13.*",
		Dockerfile:     "docker/Dockerfile",
		Dependencies:   []string{"apache-airflow==3.1.*", "pandas"},
		Packages:       []string{"libpq-dev"},
	}
	v := reflect.ValueOf(want)
	for i := range v.NumField() {
		if v.Field(i).IsZero() {
			t.Fatalf("this test leaves ManifestBuild.%s zero; set it, so the round trip covers it",
				v.Type().Field(i).Name)
		}
	}

	var p Plan
	p.SetManifestBuild(want)
	if got := p.ManifestBuild(want.ProjectDir); !reflect.DeepEqual(got, want) {
		t.Fatalf("Plan.SetManifestBuild then Plan.ManifestBuild = %+v, want %+v: "+
			"a ManifestBuild field is missing from one of the two (pkg/localrt/rt/imagebuilder.go)", got, want)
	}
}
