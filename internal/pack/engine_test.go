package pack

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

// The working tag the build lands under is dropped once both final tags point
// at the image, and only then, so the package leaves astro-package/<name>'s
// content-addressed tag and :latest behind and nothing else.
func TestAstroBuildDropsTheWorkingTagLast(t *testing.T) {
	builder := &fakeBuilder{}
	docker := &fakeDocker{inspectOut: "3.1-2"}
	res, err := newAstro(builder, docker).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)

	working := builder.gotReq.Tag
	require.True(t, strings.HasPrefix(working, "astro-package/my-project:src-"), working)
	calls := docker.callStrings()
	require.GreaterOrEqual(t, len(calls), 3)
	assert.Equal(t, []string{
		"docker tag " + working + " " + res.Image,
		"docker tag " + working + " astro-package/my-project:latest",
		"docker rmi " + working,
	}, calls[len(calls)-3:])
}

// A builder that returns a reference other than the working tag never has it
// untagged: that name is not package's to drop.
func TestAstroBuildLeavesAnotherReturnedReferenceTagged(t *testing.T) {
	docker := &fakeDocker{inspectOut: "3.1-2"}
	_, err := newAstro(&fakeBuilder{returns: "someone/else:1"}, docker).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.False(t, hasCall(docker.callStrings(), "rmi"), "%v", docker.callStrings())
}

// An inspect that fails says nothing about the image's base: the tag falls
// back to the manifest pin and a declared build is not reported as off Astro
// Runtime.
func TestAstroBuildWithAFailedInspectDoesNotWarnAboutTheBase(t *testing.T) {
	req := declaringRequest(t, "Dockerfile", "FROM python:3.12-slim\n")
	var lines []string
	cb := localrt.Callbacks{OnLine: func(l localrt.LogLine) { lines = append(lines, l.Text) }}

	res, err := newAstro(&fakeBuilder{}, &fakeDocker{failVerb: "image"}).Build(context.Background(), req, cb)
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(lines, "\n"), "not based on Astro Runtime")
	assert.Equal(t, req.Manifest.Airflow().Pin, res.RuntimeVersion)
}

// --save streams the engine's progress under the package component, and a
// failed save names the image and the path.
func TestAstroBuildSaveStreamsProgressAndNamesAFailure(t *testing.T) {
	req := testRequest(t)
	req.Save = filepath.Join(t.TempDir(), "image.tar")
	var lines []localrt.LogLine
	cb := localrt.Callbacks{OnLine: func(l localrt.LogLine) { lines = append(lines, l) }}

	_, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2", saveSays: "Saving layer 1/2\n"}).Build(context.Background(), req, cb)
	require.NoError(t, err)
	require.NotEmpty(t, lines)
	last := lines[len(lines)-1]
	assert.Equal(t, "package", last.Component)
	assert.Equal(t, "Saving layer 1/2", last.Text)

	_, err = newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2", failVerb: "save"}).Build(context.Background(), req, localrt.Callbacks{})
	require.ErrorContains(t, err, "saving astro-package/my-project:")
	assert.ErrorContains(t, err, " to "+req.Save+": docker save ")
}

// A tag that fails stops the package with the engine's own failure.
func TestAstroBuildTagFailureIsTheEngines(t *testing.T) {
	_, err := newAstro(&fakeBuilder{}, &fakeDocker{inspectOut: "3.1-2", failVerb: "tag"}).Build(context.Background(), testRequest(t), localrt.Callbacks{})
	require.ErrorContains(t, err, "docker tag astro-package/my-project:src-")
}
