package imagebuild

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// noCatalog is catalog options under which a fetch cannot succeed: none of the
// cases below should read the catalog, and one that did fails rather than
// passing on what the network said.
var noCatalog = runtimeversions.Options{Timeout: time.Nanosecond}

// Local Docker mode and deploy pick the same build for the same Airflow 3
// manifest: a declared Dockerfile, a [tool.astro] runtime build, and the
// default generated build over the pin's series. Each side is checked against
// the fixed expectation rather than against the other.
func TestForLocalManifestPicksWhatForManifestPicksForAirflow3(t *testing.T) {
	deps, pkgs := []string{"pandas>=2"}, []string{"libpq-dev"}
	declared := declaredProject(t)
	cases := map[string]struct {
		m    ManifestBuild
		want Request
	}{
		"declared dockerfile": {
			m: ManifestBuild{ProjectDir: declared, AirflowVersion: "3.1", Dockerfile: "docker/Dockerfile"},
			want: Request{
				Dockerfile: filepath.Join(declared, "docker", "Dockerfile"),
				Context:    declared,
			},
		},
		"runtime pinned": {
			m:    ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.3.1", Runtime: "3.3-8"},
			want: Request{BaseImage: "astrocrpublic.azurecr.io/runtime:3.3-8"},
		},
		"default": {
			m:    ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.1.2"},
			want: Request{BaseImage: "astrocrpublic.azurecr.io/runtime:3.1"},
		},
	}
	for name, tc := range cases {
		tc.m.Dependencies, tc.m.Packages = deps, pkgs
		tc.want.Dependencies, tc.want.Packages = deps, pkgs

		deploy, err := ForManifest(tc.m)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, deploy, "%s: deploy", name)

		local, err := ForLocalManifest(context.Background(), tc.m, noCatalog)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, local, "%s: local start", name)
	}
}

// A declared Dockerfile is the build whatever the pin says, so local start
// resolves no base for it either: an Airflow 2 pin beside it would need the
// catalog, and noCatalog cannot be read.
func TestForLocalManifestResolvesNoBaseForADeclaredDockerfile(t *testing.T) {
	dir := declaredProject(t)
	req, err := ForLocalManifest(context.Background(),
		ManifestBuild{ProjectDir: dir, AirflowVersion: "2.10.5", Dockerfile: "docker/Dockerfile"}, noCatalog)
	require.NoError(t, err)
	assert.True(t, req.FromDeclaredDockerfile())
	assert.Empty(t, req.BaseImage)
}

// The one difference is deliberate: deploy refuses Airflow 2, and local Docker
// mode runs it. A named runtime build needs no catalog lookup.
func TestForLocalManifestAlsoRunsAirflow2(t *testing.T) {
	m := ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "2.10.5", Runtime: "12.9.0"}
	_, err := ForManifest(m)
	assert.ErrorContains(t, err, "only Airflow 3")

	req, err := ForLocalManifest(context.Background(), m, noCatalog)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:12.9.0", req.BaseImage)
}

// A pin the local rule refuses stops the pick, as deploy's does.
func TestForLocalManifestRefusesAnAirflow2PinBelowTheFloor(t *testing.T) {
	_, err := ForLocalManifest(context.Background(),
		ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "2.6.3", Runtime: "8.8.0"}, noCatalog)
	assert.ErrorContains(t, err, "Airflow 2.7 or later")
}
