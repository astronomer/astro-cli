package imagebuild

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// ForManifestWith resolves a generated build's base through the resolver it is
// handed, and is otherwise ForManifest: the image ships, so it carries the
// project's files.
func TestForManifestWithUsesTheGivenBase(t *testing.T) {
	dir := t.TempDir()
	var asked []ManifestBuild
	req, err := ForManifestWith(context.Background(), ManifestBuild{ProjectDir: dir, AirflowVersion: "2.10", Runtime: "12.1.0"},
		func(_ context.Context, m ManifestBuild) (string, error) {
			asked = append(asked, m)
			return "mirror.example/astro-runtime:12.1.0", nil
		})
	require.NoError(t, err)
	assert.Equal(t, "mirror.example/astro-runtime:12.1.0", req.BaseImage)
	assert.Equal(t, dir, req.ProjectFiles, "a deploy image carries the project's files")
	require.Len(t, asked, 1)
	assert.Equal(t, "12.1.0", asked[0].Runtime)

	refused := errors.New("no such runtime")
	_, err = ForManifestWith(context.Background(), ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1"},
		func(context.Context, ManifestBuild) (string, error) { return "", refused })
	require.ErrorIs(t, err, refused)

	// A declared Dockerfile names its own FROM: the resolver is not asked.
	declared := declaredProject(t)
	req, err = ForManifestWith(context.Background(), ManifestBuild{ProjectDir: declared, AirflowVersion: "3.1", Dockerfile: "docker/Dockerfile"},
		func(context.Context, ManifestBuild) (string, error) {
			t.Error("a declared Dockerfile resolves no base")
			return "", nil
		})
	require.NoError(t, err)
	assert.True(t, req.FromDeclaredDockerfile())
}

// Labels reach docker build as --label, one flag per label, in the order
// given, in both modes; and a generated build with labels is never the bare
// base, which would carry none of them.
func TestBuildPassesLabels(t *testing.T) {
	labels := []string{"io.astronomer.skip.revision=true", "io.astronomer.deploy.revision.description=a b"}
	const want = "--label io.astronomer.skip.revision=true --label io.astronomer.deploy.revision.description=a b"

	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Labels = labels
	built, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.Tag, built, "labels are built into the image, not dropped by the fast path")
	require.Len(t, cmd.calls, 1)
	assert.Contains(t, cmd.calls[0], want)

	cmd = &fakeCmd{}
	dir := declaredProject(t)
	req = testRequest(t)
	req.Dockerfile, req.Context = filepath.Join(dir, "docker", "Dockerfile"), dir
	req.Labels = labels
	_, err = testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, cmd.calls, 1)
	assert.Contains(t, cmd.calls[0], want)

	cmd = &fakeCmd{}
	_, err = testBuilder(cmd).Build(context.Background(), testRequest(t), rt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, cmd.calls, "no labels and nothing to install keeps the fast path")
}
