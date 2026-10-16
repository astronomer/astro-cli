package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// dockerfileBase decides on the final stage's image, with the rule Docker
// mode's start enforces, and resolves a FROM built from a build argument the
// way astro's own build does: from the defaults, with no --build-arg.
func TestDockerfileBase(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       baseKind
		wantRef    string
	}{
		{name: "runtime", body: "FROM astrocrpublic.azurecr.io/runtime:3.1-12\n", want: baseRuntime},
		{name: "a -base tag", body: "FROM astrocrpublic.azurecr.io/runtime:3.1-12-base\n", want: baseRuntime},
		{name: "a -python tag", body: "FROM astrocrpublic.azurecr.io/runtime:3.1-12-python-3.12\n", want: baseRuntime},
		{name: "quay astro-runtime", body: "FROM quay.io/astronomer/astro-runtime:12.1.0\n", want: baseRuntime},
		{name: "pinned by digest", body: "FROM astrocrpublic.azurecr.io/runtime@sha256:" + strings.Repeat("a", 64) + "\n", want: baseRuntime},
		{name: "a --platform flag", body: "FROM --platform=linux/amd64 astrocrpublic.azurecr.io/runtime:3.1-12\n", want: baseRuntime},
		{name: "lower-case from", body: "from astrocrpublic.azurecr.io/runtime:3.1-12\n", want: baseRuntime},

		{name: "docker in docker", body: "FROM docker:19.03.2-dind\nRUN apk add make\n", want: baseForeign, wantRef: "docker:19.03.2-dind"},
		{name: "python", body: "FROM python:3.12\n", want: baseForeign, wantRef: "python:3.12"},
		{name: "OSS Airflow", body: "FROM apache/airflow:3.0.1\n", want: baseForeign},
		{name: "a private image named runtime", body: "FROM myco/our-runtime:3.1-12\n", want: baseForeign},
		{name: "a lookalike registry", body: "FROM quay.io/astronomerfake/astro-runtime:12.1.0\n", want: baseForeign},

		{
			name: "multi-stage, runtime last",
			body: "FROM python:3.12-slim AS builder\nRUN pip wheel x\nFROM astrocrpublic.azurecr.io/runtime:3.1-12\n",
			want: baseRuntime,
		},
		{
			name: "multi-stage, runtime first",
			body: "FROM astrocrpublic.azurecr.io/runtime:3.1-12 AS airflow\nFROM python:3.12-slim\nCOPY --from=airflow / /\n",
			want: baseForeign,
		},
		{
			name: "final stage names an earlier one",
			body: "FROM astrocrpublic.azurecr.io/runtime:3.1-12 AS base\nFROM base\nRUN true\n",
			want: baseRuntime,
		},

		{
			name: "build argument defaulting to runtime",
			body: "ARG BASE=astrocrpublic.azurecr.io/runtime:3.1-12\nFROM ${BASE}\n",
			want: baseRuntime,
		},
		{
			name: "build argument defaulting to runtime, quoted",
			body: "ARG BASE=\"astrocrpublic.azurecr.io/runtime:3.1-12\"\nFROM $BASE\n",
			want: baseRuntime,
		},
		{
			name: "build argument for the tag only",
			body: "ARG TAG=3.1-12\nFROM astrocrpublic.azurecr.io/runtime:${TAG}\n",
			want: baseRuntime,
		},
		{
			name: "build argument for the registry, with an inline default",
			body: "ARG REGISTRY\nFROM ${REGISTRY:-astrocrpublic.azurecr.io}/runtime:3.1-12\n",
			want: baseRuntime,
		},
		{
			name:    "build argument defaulting to python",
			body:    "ARG BASE=python:3.12\nFROM ${BASE}\nRUN true\n",
			want:    baseForeign,
			wantRef: "${BASE} (python:3.12 by default)",
		},
		{
			name: "inline default of python",
			body: "FROM ${BASE:-python:3.12}\n",
			want: baseForeign,
		},
		{
			// Docker expands an argument with no default to empty, and astro
			// passes no --build-arg, so this is a base Docker mode cannot build.
			name:    "build argument with no default",
			body:    "ARG BASE\nFROM ${BASE}\n",
			want:    baseForeign,
			wantRef: "${BASE} (a build argument with no default)",
		},
		{
			// Only the ARGs before the first FROM reach a FROM.
			name: "stage-scoped argument",
			body: "FROM python:3.12 AS one\nARG BASE=astrocrpublic.azurecr.io/runtime:3.1-12\nFROM ${BASE}\n",
			want: baseForeign,
		},
		{
			name:    "a modifier this does not expand",
			body:    "ARG BASE=astrocrpublic.azurecr.io/runtime:3.1-12\nFROM ${BASE:?unset}\n",
			want:    baseUnknown,
			wantRef: "${BASE:?unset}",
		},
		{name: "no FROM", body: "RUN true\n", want: baseUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), fileDockerfile)
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))

			kind, ref := dockerfileBase(path, []byte(tc.body))
			assert.Equal(t, tc.want, kind)
			if tc.wantRef != "" {
				assert.Equal(t, tc.wantRef, ref)
			}
		})
	}
}

// initWith runs init in a fresh directory holding files, and returns the
// directory, the result and the manifest it wrote.
func initWith(t *testing.T, files map[string]string) (string, *Result, *manifest.Manifest) {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, files)
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	m, err := manifest.Parse(data)
	require.NoError(t, err)
	return dir, res, m
}

// dockerfileNotes are the notes about the Dockerfile.
func dockerfileNotes(res *Result) []string {
	var out []string
	for _, n := range res.Notes {
		if strings.HasPrefix(n, fileDockerfile+":") {
			out = append(out, n)
		}
	}
	return out
}

// A Dockerfile that does not build on an Astro Runtime image is not this
// project's build. Running init in a repo that has one — the astro-cli repo's
// own is FROM docker:dind — declared it, and Docker mode then refuses to start.
// The file stays as it was, the manifest does not name it, init still
// scaffolds, and one note says why and how to declare one.
func TestInitDoesNotDeclareADockerfileOnAnotherBase(t *testing.T) {
	for _, tc := range []struct{ name, body, base string }{
		{
			name: "docker in docker",
			body: "FROM docker:19.03.2-dind\nRUN apk add --no-cache make git\n" +
				"RUN --mount=type=secret,id=token cat /run/secrets/token\nRUN pip install pytest\n",
			base: "docker:19.03.2-dind",
		},
		{name: "python with steps", body: "FROM python:3.12\nCOPY . /app\nENV X=1\n", base: "python:3.12"},
		{name: "python alone", body: "FROM python:3.12\n", base: "python:3.12"},
		{
			name: "multi-stage ending on python",
			body: "FROM astrocrpublic.azurecr.io/runtime:3.1-12 AS airflow\nFROM python:3.12-slim\n",
			base: "python:3.12-slim",
		},
		{name: "a build argument defaulting to python", body: "ARG BASE=python:3.12\nFROM ${BASE}\nRUN true\n", base: "python:3.12"},
	} {
		for _, arm := range []struct {
			name  string
			files map[string]string
		}{
			{name: "greenfield", files: map[string]string{}},
			{name: "adopt", files: map[string]string{manifest.Marker: "[project]\nname = \"repo\"\nversion = \"0.1.0\"\n"}},
		} {
			t.Run(tc.name+"/"+arm.name, func(t *testing.T) {
				files := map[string]string{fileDockerfile: tc.body, "requirements.txt": "pandas==2.1.0\n"}
				for k, v := range arm.files {
					files[k] = v
				}
				dir, res, m := initWith(t, files)

				assert.Empty(t, m.Astro.Dockerfile, "a Dockerfile Docker mode cannot start must not become the build")
				got, err := os.ReadFile(filepath.Join(dir, fileDockerfile))
				require.NoError(t, err)
				assert.Equal(t, tc.body, string(got), "the Dockerfile is left as it was")
				assert.NoFileExists(t, filepath.Join(dir, fileDockerignore), "the ignore file is for a declared build")
				assert.FileExists(t, filepath.Join(dir, "dags", "exampledag.py"), "init still scaffolds")
				assert.Contains(t, strings.Join(m.Project.Dependencies, " "), "pandas==2.1.0")
				assert.NotContains(t, strings.Join(res.Created, " ")+strings.Join(res.Updated, " "),
					"declared "+fileDockerfile)

				notes := dockerfileNotes(res)
				require.Len(t, notes, 1, "told once: %q", notes)
				assert.Equal(t, foreignDockerfileNote(notesBase(notes[0])), notes[0])
				assert.Contains(t, notes[0], tc.base)
				assert.Contains(t, notes[0], "not an Astro Runtime image")
				assert.Contains(t, notes[0], "dockerfile = 'Dockerfile' under [tool.astro]")
				for _, n := range res.Notes {
					assert.NotContains(t, n, "kept, because your Dockerfile's base image reads it")
				}
			})
		}
	}
}

// notesBase recovers the ref foreignDockerfileNote was given, so a test can
// compare the whole note without restating it.
func notesBase(note string) string {
	_, rest, _ := strings.Cut(note, "builds on ")
	ref, _, _ := strings.Cut(rest, ", which is not an Astro Runtime image")
	return ref
}

// A Dockerfile on an Astro Runtime image that does more than name it is
// declared, as before, whatever the registry or flavor.
func TestInitDeclaresADockerfileOnARuntimeBase(t *testing.T) {
	for _, from := range []string{
		"astrocrpublic.azurecr.io/runtime:3.1-12",
		"astrocrpublic.azurecr.io/runtime:3.1-12-base",
		"astrocrpublic.azurecr.io/runtime:3.1-12-python-3.12",
		"--platform=linux/amd64 astrocrpublic.azurecr.io/runtime:3.1-12",
		"quay.io/astronomer/astro-runtime:12.1.0",
	} {
		t.Run(from, func(t *testing.T) {
			body := "FROM " + from + "\nENV TZ=UTC\n"
			dir, res, m := initWith(t, map[string]string{fileDockerfile: body})

			assert.Equal(t, fileDockerfile, m.Astro.Dockerfile)
			got, err := os.ReadFile(filepath.Join(dir, fileDockerfile))
			require.NoError(t, err)
			assert.Equal(t, body, string(got))
			for _, n := range res.Notes {
				assert.NotContains(t, n, "not an Astro Runtime image")
			}
		})
	}
}

// A FROM built from a build argument is decided by its default, which is what
// astro's build uses. One in a form this does not expand is declared, as
// every Dockerfile was before the check, with a note saying it was not read.
func TestInitDecidesABuildArgumentBaseByItsDefault(t *testing.T) {
	t.Run("defaulting to runtime", func(t *testing.T) {
		_, res, m := initWith(t, map[string]string{
			fileDockerfile: "ARG BASE=astrocrpublic.azurecr.io/runtime:3.1-12\nFROM ${BASE}\nENV TZ=UTC\n",
		})
		assert.Equal(t, fileDockerfile, m.Astro.Dockerfile)
		joined := strings.Join(res.Notes, "\n")
		assert.NotContains(t, joined, "no stage builds on an Astro Runtime image")
		assert.NotContains(t, joined, "not an Astro Runtime image")
		assert.Contains(t, joined, "an Astro Runtime image named by a build argument")
	})
	t.Run("with no default", func(t *testing.T) {
		_, res, m := initWith(t, map[string]string{fileDockerfile: "ARG BASE\nFROM ${BASE}\nRUN true\n"})
		assert.Empty(t, m.Astro.Dockerfile)
		assert.Contains(t, strings.Join(res.Notes, "\n"), "${BASE} (a build argument with no default)")
	})
	t.Run("not expanded", func(t *testing.T) {
		_, res, m := initWith(t, map[string]string{
			fileDockerfile: "ARG BASE=astrocrpublic.azurecr.io/runtime:3.1-12\nFROM ${BASE:?unset}\nENV TZ=UTC\n",
		})
		assert.Equal(t, fileDockerfile, m.Astro.Dockerfile)
		joined := strings.Join(res.Notes, "\n")
		assert.Contains(t, joined, "${BASE:?unset}")
		assert.Contains(t, joined, "remove dockerfile under [tool.astro] in pyproject.toml")
	})
}

// A 1.x project's Dockerfile builds on Astro Runtime, and its conversion is
// what it was: declared, with requirements.txt kept for the base's ONBUILD. A
// 1.x layout on another base gets the same refusal as any directory, and is
// still a 1.x project, so APC still refuses to convert it.
func TestConvert1xDockerfileByBase(t *testing.T) {
	t.Run("runtime", func(t *testing.T) {
		dir, res, m := initWith(t, map[string]string{
			fileDockerfile:       "FROM quay.io/astronomer/astro-runtime:12.1.0\nENV FOO=bar\n",
			".astro/config.yaml": "project:\n  name: orders\n",
			"requirements.txt":   "pandas==2.1.0\n",
		})
		assert.Equal(t, fileDockerfile, m.Astro.Dockerfile)
		assert.FileExists(t, filepath.Join(dir, "requirements.txt"))
		assert.Contains(t, strings.Join(res.Notes, "\n"), "requirements.txt: kept, because your Dockerfile's base image reads it")
	})
	foreign := map[string]string{
		fileDockerfile:       "FROM apache/airflow:2.9.0\nUSER airflow\n",
		".astro/config.yaml": "project:\n  name: orders\n",
		"requirements.txt":   "pandas==2.1.0\n",
	}
	t.Run("another base", func(t *testing.T) {
		dir, res, m := initWith(t, foreign)
		assert.Empty(t, m.Astro.Dockerfile)
		assert.FileExists(t, filepath.Join(dir, fileDockerfile))
		require.Len(t, dockerfileNotes(res), 1)
		assert.Contains(t, dockerfileNotes(res)[0], "apache/airflow:2.9.0, which is not an Astro Runtime image")
	})
	t.Run("another base under APC", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, foreign)
		require.True(t, Is1xProject(dir))
		_, err := Plan(dir, Options{DeploysToAPC: true})
		require.ErrorIs(t, err, ErrConvert1xUnderAPC)
	})
}
