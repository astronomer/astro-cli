package pack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The image is built and handled with the engine resolved for the project
// being packaged. The target used to hard-code "docker", so container.binary:
// podman built the package with docker anyway, or failed on a host with only
// podman.
func TestAstroBuildUsesTheEngineResolvedForTheProject(t *testing.T) {
	builder := &fakeBuilder{}
	docker := &fakeDocker{inspectOut: "3.1-2\n"}
	req := testRequest(t)
	var askedFor string
	target := NewAstroTarget(builder, docker, func(projectDir string) (string, []string, error) {
		askedFor = projectDir
		return "podman", []string{"CONTAINER_HOST=unix:///tmp/m.sock"}, nil
	})

	_, err := target.Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	assert.Equal(t, req.ProjectDir, askedFor)
	assert.Equal(t, "podman", builder.gotReq.Bin)
	assert.Equal(t, []string{"CONTAINER_HOST=unix:///tmp/m.sock"}, builder.gotReq.Env)
	docker.mu.Lock()
	defer docker.mu.Unlock()
	require.NotEmpty(t, docker.calls)
	for _, call := range docker.calls {
		assert.Equal(t, "podman", call[0], "every probe, inspect, tag and untag runs the resolved CLI: %v", call)
	}
}

// Podman with no machine up names the podman fix rather than ErrNoDocker's
// "start Docker", and runs nothing.
func TestAstroBuildWithNoPodmanMachineNamesThePodmanFix(t *testing.T) {
	docker := &fakeDocker{}
	target := NewAstroTarget(&fakeBuilder{}, docker, func(string) (string, []string, error) {
		return "", nil, fmt.Errorf("%w; start one with `podman machine start`", container.ErrMachineNotRunning)
	})

	_, err := target.Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.ErrorIs(t, err, container.ErrMachineNotRunning)
	assert.NotErrorIs(t, err, ErrNoDocker)
	assert.Contains(t, err.Error(), "podman machine start")
	assert.Empty(t, docker.calls)
}

// No engine installed is the same plain error as an engine that is down.
func TestAstroBuildWithNoEngineIsErrNoDocker(t *testing.T) {
	docker := &fakeDocker{}
	target := NewAstroTarget(&fakeBuilder{}, docker, func(string) (string, []string, error) {
		return "", nil, errors.New("no container runtime")
	})

	_, err := target.Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.ErrorIs(t, err, ErrNoDocker)
	assert.Empty(t, docker.calls, "nothing runs without an engine")
}
