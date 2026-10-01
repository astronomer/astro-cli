package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	localrt "github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// noCatalog is catalog options under which a fetch cannot succeed. None of the
// cases here should read the catalog.
var noCatalog = runtimeversions.Options{Timeout: time.Nanosecond}

// `astro local start --docker` picks the image `astro deploy` and `astro
// package astro` pick for the same manifest. Both sides are checked against a
// fixed expectation, not against each other.
func TestDockerStartPicksTheImageDeployPicks(t *testing.T) {
	declared := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(declared, "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\n"), 0o600))
	deps, pkgs := []string{"pandas>=2"}, []string{"libpq-dev"}

	cases := map[string]struct {
		m    imagebuild.ManifestBuild
		want localrt.BuildRequest
	}{
		"declared dockerfile": {
			m:    imagebuild.ManifestBuild{ProjectDir: declared, AirflowVersion: "3.1", Dockerfile: "Dockerfile"},
			want: localrt.BuildRequest{Dockerfile: filepath.Join(declared, "Dockerfile"), Context: declared},
		},
		"runtime pinned": {
			m:    imagebuild.ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.3", Runtime: "3.3-8"},
			want: localrt.BuildRequest{BaseImage: "astrocrpublic.azurecr.io/runtime:3.3-8"},
		},
		"default": {
			m:    imagebuild.ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.1.2"},
			want: localrt.BuildRequest{BaseImage: "astrocrpublic.azurecr.io/runtime:3.1"},
		},
	}
	start := imageBuilder{catalog: noCatalog}
	for name, tc := range cases {
		tc.m.Dependencies, tc.m.Packages = deps, pkgs
		tc.want.Dependencies, tc.want.Packages = deps, pkgs

		got, err := start.Request(context.Background(), tc.m)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, got, "%s: start", name)

		deploy, err := imagebuild.ForManifest(tc.m)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, localrt.BuildRequest{
			BaseImage:    deploy.BaseImage,
			Dockerfile:   deploy.Dockerfile,
			Context:      deploy.Context,
			Dependencies: deploy.Dependencies,
			Packages:     deploy.Packages,
		}, "%s: deploy", name)
	}
}

// Docker start runs Airflow 2, which deploy refuses: the adapter takes the
// local rule, not deploy's.
func TestDockerStartRunsAirflow2(t *testing.T) {
	start := imageBuilder{catalog: noCatalog}
	got, err := start.Request(context.Background(),
		imagebuild.ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "2.10.5", Runtime: "12.9.0"})
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:12.9.0", got.BaseImage)
}
