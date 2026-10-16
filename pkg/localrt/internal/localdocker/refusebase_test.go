package localdocker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

func writeDockerfile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Dockerfile")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	return path
}

// labeled answers the runtime label inspect with v for every image.
func labeled(v string) func(string) (string, error) {
	return func(string) (string, error) { return v, nil }
}

// declaredStart is what startDeclared saw: the start's error, the commands the
// engine ran, the builds it asked for and the plan it started.
type declaredStart struct {
	err    error
	cmd    *fakeCmd
	images *stubImages
	plan   rt.Plan
}

// startDeclared starts a project whose declared Dockerfile is body, against a
// fake engine whose built image carries label ("" for none).
func startDeclared(t *testing.T, body, label string) declaredStart {
	t.Helper()
	cmd := &fakeCmd{output: noProjects, runtimeLabel: labeled(label)}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)

	p := testPlan(t)
	require.NoError(t, os.WriteFile(filepath.Join(p.ProjectPath, "Dockerfile"), []byte(body), 0o600))
	p.Dockerfile = "Dockerfile"

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	return declaredStart{err: err, cmd: cmd, images: images, plan: p}
}

func ranAny(calls []string, substr string) bool {
	for _, c := range calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// A mirror of a Runtime image is a Runtime image. Its name is a private
// registry's, which the old list of Astronomer's registries refused, and the
// label it kept is what says what it is. Accepted silently: nothing about the
// start differs from one on the published name.
func TestStartAcceptsAMirroredRuntimeByItsLabel(t *testing.T) {
	got := startDeclared(t,
		"FROM registry.corp.example/astronomer/astro-runtime:12.1.0\nRUN echo hi\n", "12.1.0")
	cmd, err := got.cmd, got.err

	require.NoError(t, err)
	assert.True(t, ranAny(cmd.calls, "up --detach"), "the stack should have come up; calls: %v", cmd.calls)
	assert.False(t, ranAny(cmd.calls, "image rm"), "an accepted image is the one that runs; nothing should remove it")
}

// A base without the label is refused, after the build and before compose, and
// the refusal is the plain reason: the base as the FROM wrote it, and what it
// lacks.
func TestStartRefusesABaseWithoutTheRuntimeLabel(t *testing.T) {
	got := startDeclared(t, "FROM python:3.12-slim\nRUN echo hi\n", "")
	cmd, images, p, err := got.cmd, got.images, got.plan, got.err

	require.Error(t, err)
	assert.Equal(t,
		"python:3.12-slim is not an Astro Runtime image: it has no runtime version label (io.astronomer.docker.runtime.version)",
		err.Error())
	assert.ErrorIs(t, err, airflowrt.ErrUnsupportedBase, "cmd/local and Astro Desktop name the refusal by this sentinel")
	var reason *airflowrt.NotRuntimeImageError
	require.ErrorAs(t, err, &reason)
	assert.Equal(t, "python:3.12-slim", reason.Image)

	require.Len(t, images.requests, 1, "the label is read off the built image, so the build has to have run")
	assert.False(t, ranAny(cmd.calls, "up --detach"), "a refused image must not reach compose; calls: %v", cmd.calls)
	name, nameErr := composeProjectName(p.ProjectPath)
	require.NoError(t, nameErr)
	assert.True(t, ranAny(cmd.calls, "image rm --force "+builtImageTag(name)),
		"the refused image is this start's and nothing can run it; calls: %v", cmd.calls)
}

// The final stage decides, because that is the image that runs, and it is the
// one the refusal names.
func TestStartRefusalNamesTheFinalStage(t *testing.T) {
	err := startDeclared(t,
		"FROM astrocrpublic.azurecr.io/runtime:3.1-12 AS base\nFROM apache/airflow:2.9.3\n", "").err

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "apache/airflow:2.9.3 is not an Astro Runtime image"), err.Error())
}

// A FROM built from a build argument names nothing the file can show, so the
// name-based check let it through unjudged. The image it built can be asked.
func TestStartRefusesABuildArgumentFROMOnANonRuntimeBase(t *testing.T) {
	got := startDeclared(t, "ARG BASE=python:3.12-slim\nFROM ${BASE}\n", "")
	images, err := got.images, got.err

	require.Error(t, err)
	assert.ErrorIs(t, err, airflowrt.ErrUnsupportedBase)
	assert.Equal(t,
		"the image built from Dockerfile is not an Astro Runtime image: it has no runtime version label (io.astronomer.docker.runtime.version)",
		err.Error(), "with no base in the file to name, the refusal names the file as the manifest does")
	assert.Len(t, images.requests, 1)
}

// An Astronomer name is not enough either: the image is asked, not its name.
func TestStartRefusesAnAstronomerNamedImageWithoutTheLabel(t *testing.T) {
	err := startDeclared(t, "FROM quay.io/astronomer/ap-airflow:2.5.1\n", "").err
	assert.ErrorIs(t, err, airflowrt.ErrUnsupportedBase)
}

// An inspect that fails says nothing about the base, so it is not reported as
// a base without the label.
func TestStartReportsAFailedLabelReadAsItself(t *testing.T) {
	cmd := &fakeCmd{output: noProjects, runtimeLabel: func(string) (string, error) {
		return "", errors.New("daemon went away")
	}}
	e := testEngine(t, cmd)
	p := dockerfilePlan(t)

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, airflowrt.ErrUnsupportedBase)
	assert.Contains(t, err.Error(), "daemon went away")
	assert.Contains(t, err.Error(), "reading the runtime version label")
}

// A generated build is not inspected. Its base is the runtime image the pin
// resolved, and with nothing to install that base is what runs, not yet pulled
// into the local store for an inspect to find.
func TestStartDoesNotInspectAGeneratedBuild(t *testing.T) {
	cmd := &fakeCmd{output: noProjects, runtimeLabel: labeled("")}
	e := testEngine(t, cmd)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err)
	assert.False(t, ranAny(cmd.calls, "image inspect"), "calls: %v", cmd.calls)
}

// The refusal is reported the way every other failed start is: starting, then
// error with the refusal, so a consumer streaming states sees the reason.
func TestStartReportsTheRefusalAsAnErrorState(t *testing.T) {
	cmd := &fakeCmd{output: noProjects, runtimeLabel: labeled("")}
	e := testEngine(t, cmd)
	p := testPlan(t)
	require.NoError(t, os.WriteFile(filepath.Join(p.ProjectPath, "Dockerfile"),
		[]byte("FROM apache/airflow:2.9.3\n"), 0o600))
	p.Dockerfile = "Dockerfile"

	var states []rt.State
	var reported error
	_, err := e.Start(context.Background(), p, rt.Callbacks{
		OnState: func(s rt.State, err error) {
			states = append(states, s)
			if s == rt.StateError {
				reported = err
			}
		},
	})

	require.ErrorIs(t, err, airflowrt.ErrUnsupportedBase)
	assert.Equal(t, []rt.State{rt.StateStarting, rt.StateError}, states)
	assert.ErrorIs(t, reported, airflowrt.ErrUnsupportedBase)
}

// The inspect reads the one label, and treats docker's "<no value>" for an
// absent one as no label rather than as a version.
func TestReadRuntimeLabel(t *testing.T) {
	for _, tc := range []struct {
		name, out, want string
	}{
		{"present", "3.1-17\n", "3.1-17"},
		{"absent", "<no value>\n", ""},
		{"empty", "\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var asked string
			cmd := &fakeCmd{output: func(call string) ([]byte, error) {
				asked = call
				return []byte(tc.out), nil
			}}
			e := testEngine(t, cmd)
			// Through the generic hook, not fakeCmd's own label answer, so the
			// command line itself is what is checked.
			got, err := e.readRuntimeLabel(context.Background(), engineConn{bin: "podman"}, "astro-local/x")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t,
				`podman image inspect --format {{ index .Config.Labels "io.astronomer.docker.runtime.version" }} astro-local/x`,
				asked)
		})
	}
}

// Before a build there is no image to ask, and the FROM tag is the fallback.
// A digest-pinned runtime takes its generation from the pin: the reference
// names an exact image and no version, so there is nothing in it to read.
// Reading the digest as a tag made every digest-pinned project Airflow 2.
func TestADigestPinnedRuntimeFollowsThePin(t *testing.T) {
	path := writeDockerfile(t, "FROM astrocrpublic.azurecr.io/runtime@sha256:abc123\n")
	base := airflowrt.ReadDeclaredBase(path)
	require.True(t, base.Known)

	assert.Equal(t, "3", planMajor("3.1", base.RuntimeVersion()), "a digest carries no generation, so the pin decides")
	assert.Equal(t, "2", planMajor("2.10.5", base.RuntimeVersion()))
}

// The fallback reads the FROM's runtime version, flavor stripped, when the base
// is named as an Astro Runtime image with a tag, and the pin otherwise.
func TestPlanMajorFallsBackToTheDeclaredBase(t *testing.T) {
	cases := []struct {
		from, pin, want string
	}{
		{from: "astrocrpublic.azurecr.io/runtime:3.3-8", pin: "2.10", want: "3"},
		{from: "astrocrpublic.azurecr.io/runtime:3.3-8-python-3.12", pin: "2.10", want: "3"},
		{from: "quay.io/astronomer/astro-runtime:13.11.0", pin: "3.3", want: "2"},
		{from: "quay.io/astronomer/astro-runtime:13.7.0-slim", pin: "3.3", want: "2"},
		// A generation-only tag reads by the check's grammar, not the loose
		// "3." prefix rule, which would call it Airflow 2.
		{from: "astrocrpublic.azurecr.io/runtime:3", pin: "2.10", want: "3"},
		// Not a runtime version: the loose rule still reads Airflow 2.
		{from: "quay.io/astronomer/ap-airflow:2.5.1", pin: "3.3", want: "2"},
		// A name that is not Astronomer's says nothing before the build.
		{from: "registry.corp.example/astronomer/astro-runtime:12.1.0", pin: "3.3", want: "3"},
		{from: "python:3.12-slim", pin: "3.3", want: "3"},
		{from: "python:3.12-slim", pin: "2.10", want: "2"},
	}
	for _, tc := range cases {
		base := airflowrt.ReadDeclaredBase(writeDockerfile(t, "FROM "+tc.from+"\n"))
		assert.Equal(t, tc.want, planMajor(tc.pin, base.RuntimeVersion()), "FROM %s beside %s", tc.from, tc.pin)
	}
}

// The label read off a built image takes its generation by the same grammar.
func TestPlanMajorReadsARuntimeLabel(t *testing.T) {
	for label, want := range map[string]string{
		"3.1-17":  "3",
		"3.0-1":   "3",
		"12.1.0":  "2",
		"13.11.0": "2",
	} {
		assert.Equal(t, want, planMajor("9.9", label), "label %s", label)
	}
}
