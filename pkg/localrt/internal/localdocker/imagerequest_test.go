package localdocker

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// The engine asks the image builder which build the plan's manifest fields
// pick rather than choosing for itself: every field that decides the image
// reaches the seam, with the project resolved to an absolute path.
func TestStartAsksTheImageBuilderWhichBuildThePlanPicks(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)
	p := testPlan(t)
	p.Runtime = "3.3-8"
	p.Dependencies = []string{"pandas"}
	p.Packages = []string{"libpq-dev"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	abs, err := filepath.Abs(p.ProjectPath)
	require.NoError(t, err)
	assert.Equal(t, []rt.ManifestBuild{{
		ProjectDir:     abs,
		AirflowVersion: p.AirflowVersion,
		Runtime:        "3.3-8",
		Dependencies:   []string{"pandas"},
		Packages:       []string{"libpq-dev"},
	}}, images.manifests)
}

// The request the seam picks is the one built, with the engine's own fields
// added: nothing the builder chose is replaced on the way to Build.
func TestStartBuildsTheRequestTheImageBuilderPicked(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)
	images.base = "example.test/picked:1"
	p := testPlan(t)
	p.Dependencies = []string{"pandas"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	require.Len(t, images.requests, 1)
	req := images.requests[0]
	assert.Equal(t, "example.test/picked:1", req.BaseImage)
	assert.Equal(t, []string{"pandas"}, req.Dependencies)
	assert.NotEmpty(t, req.WorkDir)
	assert.NotEmpty(t, req.Tag)
}
