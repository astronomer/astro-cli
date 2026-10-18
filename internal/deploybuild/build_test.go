package deploybuild

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/container"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// fakeCmd is an imagebuild.Commander that records calls and never touches a
// real daemon.
type fakeCmd struct {
	calls    []string
	envs     [][]string
	err      error
	buildOut string
}

func (f *fakeCmd) Run(_ context.Context, env []string, stdio localrt.Stdio, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.envs = append(f.envs, env)
	if len(args) > 0 && args[0] == "build" && f.buildOut != "" && stdio.Out != nil {
		fmt.Fprint(stdio.Out, f.buildOut)
	}
	return f.err
}

func labels(version string, err error) LabelReader {
	return func(string, string) (string, error) { return version, err }
}

func TestEnsureEngine(t *testing.T) {
	ok := func() (string, []string, error) { return "docker", []string{"X=1"}, nil }

	bin, env, err := EnsureEngine(context.Background(), &fakeCmd{}, ok)
	require.NoError(t, err)
	assert.Equal(t, "docker", bin)
	assert.Equal(t, []string{"X=1"}, env)

	_, _, err = EnsureEngine(context.Background(), &fakeCmd{}, func() (string, []string, error) { return "", nil, errors.New("none on PATH") })
	assert.ErrorContains(t, err, "needs Docker")

	_, _, err = EnsureEngine(context.Background(), &fakeCmd{err: errors.New("daemon down")}, ok)
	assert.ErrorContains(t, err, "needs Docker", "an engine whose daemon does not answer is no engine")

	_, _, err = EnsureEngine(context.Background(), &fakeCmd{}, func() (string, []string, error) {
		return "", nil, fmt.Errorf("%w; podman machine init --now", container.ErrMachineNotRunning)
	})
	require.ErrorIs(t, err, container.ErrMachineNotRunning)
	assert.Contains(t, err.Error(), "astro deploy --dags")
	assert.NotContains(t, err.Error(), "Start Docker")
}

// ReachEngine hands back the engine it probed, and words no failure: the
// caller does, so it reports only which kind of failure it was.
func TestReachEngine(t *testing.T) {
	cmd := &fakeCmd{}
	engine, err := ReachEngine(context.Background(), cmd, func() (string, []string, error) { return "podman", []string{"X=1"}, nil })
	require.NoError(t, err)
	assert.Equal(t, imagebuild.Engine{Cmd: cmd, Bin: "podman", Env: []string{"X=1"}}, engine)
	assert.Equal(t, []string{"podman info"}, cmd.calls)
	assert.Equal(t, [][]string{{"X=1"}}, cmd.envs, "the probe reaches the daemon through the resolved env")

	cmd = &fakeCmd{}
	_, err = ReachEngine(context.Background(), cmd, func() (string, []string, error) { return "", nil, errors.New("none on PATH") })
	require.ErrorIs(t, err, ErrEngineUnreachable)
	assert.Empty(t, cmd.calls, "nothing runs without an engine")

	_, err = ReachEngine(context.Background(), &fakeCmd{err: errors.New("daemon down")}, func() (string, []string, error) { return "docker", nil, nil })
	require.ErrorIs(t, err, ErrEngineUnreachable)

	machine := fmt.Errorf("%w; podman machine init --now", container.ErrMachineNotRunning)
	_, err = ReachEngine(context.Background(), &fakeCmd{}, func() (string, []string, error) { return "", nil, machine })
	require.ErrorIs(t, err, container.ErrMachineNotRunning)
	assert.Equal(t, machine, err, "the resolver's own error, which names the podman fix")
}

// A nil resolver is the caller's mistake, refused rather than a panic.
func TestEnsureEngineWithoutAResolver(t *testing.T) {
	cmd := &fakeCmd{}
	_, _, err := EnsureEngine(context.Background(), cmd, nil)
	require.ErrorIs(t, err, errNoResolver)
	assert.Empty(t, cmd.calls, "no engine was asked anything")
}

// The engine, secrets and labels on the Request reach the build as given;
// BuildImage sets only the work dir, tag and platform.
func TestBuildImage(t *testing.T) {
	dir := t.TempDir()
	cmd := &fakeCmd{buildOut: "step 1\n"}
	var progress bytes.Buffer
	image, version, err := BuildImage(context.Background(), &Image{
		Request: imagebuild.Request{
			BaseImage: imagebuild.RuntimeImageRepo + ":3.1-2",
			Secrets:   []string{"id=netrc,src=/n"},
			Labels:    []string{"a=b"},
			Bin:       "podman",
			Env:       []string{"CONTAINER_HOST=unix:///x"},
		},
		ProjectDir: dir,
		Commander:  cmd,
		Now:        func() time.Time { return time.Time{} },
		Progress:   &progress,
		ReadLabel:  labels("3.1-2", nil),
	})
	require.NoError(t, err)
	assert.Equal(t, ImageTag(dir), image)
	assert.Equal(t, "3.1-2", version)
	require.Len(t, cmd.calls, 1)
	assert.True(t, strings.HasPrefix(cmd.calls[0], "podman build --tag "+ImageTag(dir)+" "), "got %q", cmd.calls[0])
	assert.Contains(t, cmd.calls[0], "--platform linux/amd64")
	assert.Contains(t, cmd.calls[0], "--secret id=netrc,src=/n")
	assert.Contains(t, cmd.calls[0], "--label a=b")
	assert.Equal(t, [][]string{{"CONTAINER_HOST=unix:///x"}}, cmd.envs)
	assert.Equal(t, "[build] step 1\n", progress.String())
}

// A built image with no runtime label is refused, naming whose mistake it is.
func TestBuildImageWithoutTheLabel(t *testing.T) {
	dir := t.TempDir()
	in := &Image{
		Request:    imagebuild.Request{BaseImage: "example.com/base:1"},
		ProjectDir: dir,
		Commander:  &fakeCmd{},
		Now:        time.Now,
		Progress:   &bytes.Buffer{},
		ReadLabel:  labels("", nil),
	}
	_, _, err := BuildImage(context.Background(), in)
	assert.EqualError(t, err, "the built image is missing the "+airflowrt.RuntimeVersionLabel+" label; the runtime base example.com/base:1 should carry it")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o600))
	in.Request = imagebuild.Request{Dockerfile: filepath.Join(dir, "Dockerfile"), Context: dir}
	in.Dockerfile = "Dockerfile"
	_, _, err = BuildImage(context.Background(), in)
	assert.EqualError(t, err, "the image built from Dockerfile is missing the "+airflowrt.RuntimeVersionLabel+" label, so it is not based on Astro Runtime; build it FROM an Astro Runtime image")

	in.ReadLabel = labels("", errors.New("no such image"))
	_, _, err = BuildImage(context.Background(), in)
	assert.ErrorContains(t, err, "reading the runtime version off the built image: no such image")
}

func TestPrebuiltRuntime(t *testing.T) {
	v, err := PrebuiltRuntime("img:1", labels("3.1-2", nil))
	require.NoError(t, err)
	assert.Equal(t, "3.1-2", v)

	_, err = PrebuiltRuntime("img:1", labels("", errors.New("no such image")))
	assert.ErrorContains(t, err, `image "img:1" was not found locally`)

	_, err = PrebuiltRuntime("img:1", labels("", nil))
	assert.ErrorContains(t, err, `image "img:1" is not based on Astro Runtime`)
}

func TestImageTag(t *testing.T) {
	// Two same-named projects in different directories must not share a tag.
	a := filepath.Join(t.TempDir(), "proj")
	b := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(a, 0o755))
	require.NoError(t, os.MkdirAll(b, 0o755))

	tagA := ImageTag(a)
	tagB := ImageTag(b)
	assert.True(t, strings.HasPrefix(tagA, "astro-deploy/proj-"), "got %q", tagA)
	assert.True(t, strings.HasPrefix(tagB, "astro-deploy/proj-"), "got %q", tagB)
	assert.NotEqual(t, tagA, tagB, "same-named projects in different dirs must get different tags")

	// A path with no usable base name falls back to a stable label.
	assert.True(t, strings.HasPrefix(ImageTag("/"), "astro-deploy/project"), "got %q", ImageTag("/"))
}
