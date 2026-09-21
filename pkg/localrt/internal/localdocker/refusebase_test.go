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

// refuseFor runs the engine's own read-then-judge over a fixture, so these
// cases exercise the pair Start uses rather than a hand-built struct.
func refuseFor(t *testing.T, path string) error {
	t.Helper()
	return refuseUnsupportedBase(path, readDeclaredBase(path))
}

func writeDockerfile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Dockerfile")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	return path
}

// A declared Dockerfile on a base the compose file cannot run is refused.
//
// The failure without this is a daemon error about a missing unix user,
// reported to the person as "starting project containers: exit status 1" — so
// the refusal has to name the file and the base, or it is no better.
func TestStartRefusesADockerfileNotOnARuntimeBase(t *testing.T) {
	path := writeDockerfile(t, "FROM apache/airflow:2.9.3\n")

	err := refuseFor(t, path)
	if err == nil {
		t.Fatal("an apache/airflow base should be refused")
	}
	if !errors.Is(err, airflowrt.ErrUnsupportedBase) {
		t.Errorf("error does not wrap ErrUnsupportedBase, so cmd/local cannot name it: %v", err)
	}
	for _, want := range []string{path, "apache/airflow"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q, which is what the reader has to change:\n%s", want, err)
		}
	}
}

// The final stage decides, because that is the image that runs.
//
// A multi-stage build is the headline reason to declare a Dockerfile at all,
// and those open on a builder that is never an Astro image.
func TestStartJudgesTheFinalStageOfAMultiStageBuild(t *testing.T) {
	onRuntime := writeDockerfile(t,
		"FROM python:3.12-slim AS builder\nRUN echo build\nFROM astrocrpublic.azurecr.io/runtime:3.1-12\n")
	if err := refuseFor(t, onRuntime); err != nil {
		t.Errorf("a builder stage on python is normal and must not be refused: %v", err)
	}

	offRuntime := writeDockerfile(t,
		"FROM astrocrpublic.azurecr.io/runtime:3.1-12 AS base\nFROM apache/airflow:2.9.3\n")
	if err := refuseFor(t, offRuntime); err == nil {
		t.Error("the last stage is what runs, and it is not a runtime image")
	}
}

// What must NOT be refused, which is most of it.
func TestStartRefusesNothingElse(t *testing.T) {
	t.Run("no declared Dockerfile", func(t *testing.T) {
		if err := refuseFor(t, ""); err != nil {
			t.Errorf("a project with no Dockerfile has no base to judge: %v", err)
		}
	})

	t.Run("an unreadable file is left to the builder", func(t *testing.T) {
		// imagebuild.Build reports a missing or malformed Dockerfile with the
		// path and the reason. Refusing here would replace that with a guess
		// about a base nothing managed to read.
		missing := filepath.Join(t.TempDir(), "Dockerfile")
		if err := refuseFor(t, missing); err != nil {
			t.Errorf("an unreadable file is the builder's to report: %v", err)
		}
		noFrom := writeDockerfile(t, "RUN echo nothing\n")
		if err := refuseFor(t, noFrom); err != nil {
			t.Errorf("a file with no FROM is the builder's to report: %v", err)
		}
	})

	t.Run("every published runtime base", func(t *testing.T) {
		for _, base := range []string{
			"astrocrpublic.azurecr.io/runtime:3.1-12",
			"quay.io/astronomer/astro-runtime:12.9.0",
			"quay.io/astronomer/astro-runtime:12.9.0-python-3.11",
		} {
			if err := refuseFor(t, writeDockerfile(t, "FROM "+base+"\n")); err != nil {
				t.Errorf("%s is a supported base: %v", base, err)
			}
		}
	})
}

// Start refuses, and does it without waking the engine.
//
// Every case above calls the helper. None of them says the helper is WIRED,
// and the placement is half the point: deleting the call, or moving it below
// ensureEngine, left the rest of this file green. So this drives Start and
// asserts on the seam — an engine that was asked to come up means a stopped
// Docker Desktop was started to deliver an answer that was on disk.
func TestStartRefusesBeforeItTouchesTheEngine(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)

	engineWoken := false
	e.ensureEngine = func(rt.Callbacks) error {
		engineWoken = true
		return nil
	}

	p := testPlan(t)
	require.NoError(t, os.WriteFile(filepath.Join(p.ProjectPath, "Dockerfile"),
		[]byte("FROM apache/airflow:2.9.3\n"), 0o600))
	p.Dockerfile = "Dockerfile"

	var states []rt.State
	_, err := e.Start(context.Background(), p, rt.Callbacks{
		OnState: func(s rt.State, _ error) { states = append(states, s) },
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, airflowrt.ErrUnsupportedBase)
	assert.False(t, engineWoken, "the refusal is on disk; nothing should have asked the engine to start")
	assert.Empty(t, cmd.calls, "no docker command should run for a project that is refused")

	// Nothing was started, so nothing should have said it was starting: a
	// consumer seeing starting→error cannot tell this from a start that got
	// partway and may have left something behind.
	assert.NotContains(t, states, rt.StateStarting,
		"a precondition failure must not announce a start; states were %v", states)
}

// The shapes a real Dockerfile carries, judged through the same path Start uses.
//
// Each of these was refused or accepted wrongly by an earlier reading of the
// file, and every one of them is a project that builds and runs today.
func TestStartJudgesRealReferenceShapes(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantRefuse bool
	}{
		{"a platform flag before a runtime base", "FROM --platform=linux/amd64 astrocrpublic.azurecr.io/runtime:3.1-12\n", false},
		{"a templated platform flag", "FROM --platform=$BUILDPLATFORM astrocrpublic.azurecr.io/runtime:3.1-12\n", false},
		{"a tab after FROM", "FROM\tastrocrpublic.azurecr.io/runtime:3.1-12\n", false},
		{"a runtime pinned by digest", "FROM astrocrpublic.azurecr.io/runtime@sha256:abc123\n", false},
		{"a base named by a build argument", "ARG BASE=astrocrpublic.azurecr.io/runtime:3.1-12\nFROM ${BASE}\n", false},
		// Still refused, including where the old prefix match could not see it.
		{"an OSS airflow base", "FROM apache/airflow:2.9.3\n", true},
		{"an OSS airflow base after a tab", "FROM\tapache/airflow:2.9.3\n", true},
		{"a private registry with a port", "FROM localhost:5000/astro-runtime:3.1-12\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeDockerfile(t, tc.body)
			err := refuseFor(t, path)
			if tc.wantRefuse && err == nil {
				t.Errorf("%s should be refused", tc.body)
			}
			if !tc.wantRefuse && err != nil {
				t.Errorf("%s builds and runs today, and was refused: %v", tc.body, err)
			}
		})
	}
}

// A digest-pinned runtime takes its generation from the pin.
//
// The reference names an exact image and no version, so there is nothing in it
// to read. Reading the digest as a tag made every digest-pinned project Airflow
// 2 — an Airflow 3 image would then get a compose file with no api-server, which
// is the failure the refusal exists to prevent, arriving by another road.
func TestADigestPinnedRuntimeFollowsThePin(t *testing.T) {
	path := writeDockerfile(t, "FROM astrocrpublic.azurecr.io/runtime@sha256:abc123\n")
	base := readDeclaredBase(path)
	require.True(t, base.known)

	assert.Equal(t, "3", planMajor("3.1", base), "a digest carries no generation, so the pin decides")
	assert.Equal(t, "2", planMajor("2.10.5", base))
}
