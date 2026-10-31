package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
)

// Each shape the rewrite cannot take without guessing is left as it is, with a
// note naming why; a shape with nothing to rewrite is left without one.
func TestDockerfileRewriteOfGates(t *testing.T) {
	for _, tc := range []struct {
		name, body, blocker string
	}{
		{
			name:    "an earlier stage on an ONBUILD tag",
			body:    "FROM quay.io/astronomer/astro-runtime:12.1.0 AS deps\nRUN true\nFROM quay.io/astronomer/astro-runtime:12.1.0\nENV A=b\n",
			blocker: "an earlier stage builds on quay.io/astronomer/astro-runtime:12.1.0",
		},
		{
			name:    "a final stage that names an ONBUILD stage",
			body:    "FROM quay.io/astronomer/astro-runtime:12.1.0 AS base\nFROM base\nRUN true\n",
			blocker: "an earlier stage builds on quay.io/astronomer/astro-runtime:12.1.0, which astro cannot show is free",
		},
		{
			name:    "a base pinned by digest",
			body:    "FROM quay.io/astronomer/astro-runtime:12.1.0@sha256:0000000000000000000000000000000000000000000000000000000000000000\nENV A=b\n",
			blocker: "by digest",
		},
		{
			name:    "a file that reads requirements.txt itself",
			body:    "FROM quay.io/astronomer/astro-runtime:12.1.0\nRUN pip install -r requirements.txt\n",
			blocker: "because it reads requirements.txt itself",
		},
		{
			name:    "a file that reads packages.txt itself",
			body:    "FROM quay.io/astronomer/astro-runtime:12.1.0\nCOPY packages.txt /tmp/\n",
			blocker: "because it reads packages.txt itself",
		},
		{
			name:    "a final FROM continued onto the next line",
			body:    "FROM quay.io/astronomer/astro-runtime:12.1.0 \\\n  AS final\nENV A=b\n",
			blocker: "continues onto the next one",
		},
		{
			name:    "a runtime below the floor",
			body:    "FROM quay.io/astronomer/astro-runtime:7.4.0\nENV A=b\n",
			blocker: "older than runtime 9 (Airflow 2.7)",
		},
		{
			name:    "an earlier stage from a build argument",
			body:    "ARG B=quay.io/astronomer/astro-runtime:12.1.0\nFROM $B AS builder\nRUN true\nFROM quay.io/astronomer/astro-runtime:12.1.0\nENV A=b\n",
			blocker: "an earlier stage builds on $B, which astro cannot show is free",
		},
		{
			name:    "an earlier stage on a runtime digest",
			body:    "FROM quay.io/astronomer/astro-runtime@sha256:0000000000000000000000000000000000000000000000000000000000000000 AS builder\nRUN true\nFROM quay.io/astronomer/astro-runtime:12.1.0\nENV A=b\n",
			blocker: "an earlier stage builds on quay.io/astronomer/astro-runtime@sha256:",
		},
		{
			name:    "an earlier stage on a mirror's floating tag",
			body:    "FROM mirror.example/runtime:stable AS builder\nRUN true\nFROM quay.io/astronomer/astro-runtime:12.1.0\nENV A=b\n",
			blocker: "an earlier stage builds on mirror.example/runtime:stable",
		},
		{
			name:    "a floating Airflow 2 tag",
			body:    "FROM quay.io/astronomer/astro-runtime:12\nENV A=b\n",
			blocker: "the floating Airflow 2 tag 12 names no runtime build",
		},
		{
			name:    "a slim flavor",
			body:    "FROM quay.io/astronomer/astro-runtime:13.12.0-slim\nENV A=b\n",
			blocker: "names the slim flavor, which pyproject.toml has no way to pick",
		},
		{
			name:    "a ubi9 flavor",
			body:    "FROM quay.io/astronomer/astro-runtime:12.6.0-ubi9\nENV A=b\n",
			blocker: "12.6.0-ubi9",
		},
		{name: "a file that names dev-requirements.txt, not requirements.txt", body: "FROM python:3.12-slim\nCOPY dev-requirements.txt /tmp/\n"},
		{name: "a comment naming requirements.txt", body: "FROM quay.io/astronomer/astro-runtime:12.1.0-base\n# requirements.txt is gone\n"},
		{name: "a final stage on an earlier -base stage", body: "FROM quay.io/astronomer/astro-runtime:12.1.0-base AS base\nFROM base\nRUN true\n"},
		{name: "a base from a build argument", body: "ARG BASE=quay.io/astronomer/astro-runtime:12.1.0\nFROM ${BASE}\nENV A=b\n"},
		{name: "a base already on -base", body: "FROM quay.io/astronomer/astro-runtime:12.1.0-base\nENV A=b\n"},
		{name: "a file already on the install base", body: "ARG ASTRO_BASE\nFROM ${ASTRO_BASE}\nCOPY --chown=astro:0 . .\nENV A=b\n"},
		{name: "a base that is not Astro Runtime", body: "FROM python:3.12-slim\nENV A=b\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rw, blocker, _ := dockerfileRewriteOf([]byte(tc.body))
			assert.Nil(t, rw)
			if tc.blocker == "" {
				assert.Empty(t, blocker)
				return
			}
			assert.Contains(t, blocker, tc.blocker)
			assert.True(t, strings.HasPrefix(blocker, fileDockerfile+": "), "a note about the Dockerfile names it first: %q", blocker)
		})
	}
}

// A multi-stage file keeps every stage before the final one as it was, and the
// final stage keeps its name and the flags it had.
func TestDockerfileRewriteOfKeepsEarlierStagesAndTheAlias(t *testing.T) {
	body := "ARG PY=3.12\n" +
		"FROM python:3.12-slim AS builder\n" +
		"RUN pip wheel --wheel-dir /wheels mylib\n" +
		"\n" +
		"FROM --platform=linux/amd64 astrocrpublic.azurecr.io/runtime:3.1-12 AS app\n" +
		"\n" +
		"COPY --from=builder /wheels /wheels\n" +
		"ENV TZ=UTC"
	rw, blocker, _ := dockerfileRewriteOf([]byte(body))
	require.Empty(t, blocker)
	require.NotNil(t, rw)

	want := airflowrt.BaseArgCheckDirective + "\n" + airflowrt.BaseArgComment + "ARG ASTRO_BASE\n" +
		"ARG PY=3.12\nFROM python:3.12-slim AS builder\nRUN pip wheel --wheel-dir /wheels mylib\n\n" +
		"FROM --platform=linux/amd64 ${ASTRO_BASE} AS app\nCOPY --chown=astro:0 . .\n\n" + ownStepsComment +
		"COPY --from=builder /wheels /wheels\nENV TZ=UTC\n"
	assert.Equal(t, want, string(rw.body))
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1-12", rw.from)
	assert.Equal(t, "3.1-12", rw.runtime)
	assert.Empty(t, rw.registry)

	// The project's stages, the final one still named "app", on the install
	// base, and nothing left on an ONBUILD tag.
	stages := airflowrt.DockerfileStages(rw.body)
	require.Len(t, stages, 2)
	assert.Equal(t, "app", stages[1].Alias)
	assert.True(t, airflowrt.UsesInstallBase(rw.body))
	for _, st := range stages {
		assert.False(t, st.OnBuildRuntime(), "no stage is left on an ONBUILD tag: %s", st.Ref)
	}
}

// A file's own parser directives stay first, and the one the install base
// needs joins them; a file that has a check directive already keeps its own.
func TestDockerfileRewriteOfKeepsParserDirectives(t *testing.T) {
	rw, _, _ := dockerfileRewriteOf([]byte("# syntax=docker/dockerfile:1\nFROM astrocrpublic.azurecr.io/runtime:3.3\nENV A=b\n"))
	require.NotNil(t, rw)
	assert.True(t, strings.HasPrefix(string(rw.body), "# syntax=docker/dockerfile:1\n"+airflowrt.BaseArgCheckDirective+"\n# Built on"), string(rw.body))
	assert.Empty(t, rw.runtime, "a series tag is the requirement's series")

	rw, _, _ = dockerfileRewriteOf([]byte("# check=error=true\nFROM astrocrpublic.azurecr.io/runtime:3.3-8\nENV A=b\n"))
	require.NotNil(t, rw)
	assert.True(t, strings.HasPrefix(string(rw.body), "# check=error=true\n# Built on"), string(rw.body))
	assert.Equal(t, "3.3-8", rw.runtime)
}

// A conversion that rewrites says so in each list: the Dockerfile updated and
// both 1.x lists retired. Nothing is written for the build to run: the CLI
// resolves the install before it.
func TestConversionRewritesTheDockerfileAndRetiresBothLists(t *testing.T) {
	dir, res, m := initWith(t, map[string]string{
		fileDockerfile:       "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN apt-get update\n",
		".astro/config.yaml": "project:\n  name: orders\n",
		"requirements.txt":   "pandas==2.1.0\n",
		"packages.txt":       "unixodbc\n",
	})

	assert.Equal(t, fileDockerfile, m.Astro.Dockerfile)
	assert.Contains(t, m.Project.Dependencies, "pandas==2.1.0")
	assert.Equal(t, []string{"unixodbc"}, m.Astro.Packages)
	assert.NoFileExists(t, filepath.Join(dir, "requirements.txt"))
	assert.NoFileExists(t, filepath.Join(dir, "packages.txt"))
	assert.NoFileExists(t, filepath.Join(dir, staleHelperPath))

	updated := strings.Join(res.Updated, "\n")
	assert.Contains(t, updated, "Dockerfile (now builds on the install base the Astro CLI makes from pyproject.toml and uv.lock")
	assert.Equal(t, "12.6.0", m.Astro.Runtime, "the runtime the Dockerfile named moves into pyproject.toml")
	assert.Empty(t, m.Astro.Registry)
	assert.NotContains(t, strings.Join(res.Created, "\n"), ".astro/install")
	assert.Contains(t, strings.Join(res.Deleted, "\n"), "requirements.txt")
	assert.Contains(t, strings.Join(res.Deleted, "\n"), "packages.txt")
	for _, n := range res.Notes {
		assert.NotContains(t, n, "kept, because your Dockerfile's base image reads it")
	}
	assert.Empty(t, mirrorAdvisories(res), "Astronomer's own repository needs no mirroring")
}

// A requirements.txt line the manifest cannot carry keeps the file, and the
// file keeps the Dockerfile on the ONBUILD tag that reads it: rewriting would
// build an image without that line.
func TestConversionKeepsTheONBUILDTagWhenAListWasNotCarried(t *testing.T) {
	body := "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN apt-get update\n"
	dir, res, _ := initWith(t, map[string]string{
		fileDockerfile:     body,
		"requirements.txt": "--extra-index-url https://pypi.corp.example/simple\npandas==2.1.0\n",
		"packages.txt":     "unixodbc\n",
	})

	got, err := os.ReadFile(filepath.Join(dir, fileDockerfile))
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	assert.FileExists(t, filepath.Join(dir, "requirements.txt"))
	assert.FileExists(t, filepath.Join(dir, "packages.txt"), "the ONBUILD tag still reads it")

	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, "Dockerfile: kept FROM quay.io/astronomer/astro-runtime:12.6.0 rather than moved onto "+
		"the install base the Astro CLI builds, because requirements.txt has lines")
	assert.Contains(t, notes, "packages.txt: kept, because your Dockerfile's base image reads it")
}

// A mirror is recorded as [tool.astro] registry, and the run says the
// mirror has to carry the -base tag too.
func TestConversionOnAMirrorRecordsTheRegistry(t *testing.T) {
	_, res, m := initWith(t, map[string]string{
		fileDockerfile: "FROM registry.corp.example/astronomer/astro-runtime:12.6.0\nENV A=b\n",
	})
	assert.Equal(t, "registry.corp.example/astronomer", m.Astro.Registry)
	assert.Equal(t, "12.6.0", m.Astro.Runtime)
	advisories := mirrorAdvisories(res)
	require.Len(t, advisories, 1)
	assert.Contains(t, advisories[0], "[tool.astro] registry = 'registry.corp.example/astronomer'")
	assert.Contains(t, advisories[0], "mirror its -base tag too")
}

func mirrorAdvisories(res *Result) []string {
	var out []string
	for _, a := range res.Advisories {
		if strings.Contains(a, "mirror") {
			out = append(out, a)
		}
	}
	return out
}

// The install helper an earlier preview wrote is removed, with a line saying
// why; a file of the project's own at that path is not the CLI's to remove.
func TestConversionRemovesAPreviewsInstallHelper(t *testing.T) {
	files := map[string]string{
		fileDockerfile:  "FROM quay.io/astronomer/astro-runtime:12.6.0\nENV A=b\n",
		staleHelperPath: staleHelperHeader + "\nimport argparse\n",
	}
	dir, res, _ := initWith(t, files)
	assert.NoFileExists(t, filepath.Join(dir, staleHelperPath))
	assert.Contains(t, strings.Join(res.Deleted, "\n"), staleHelperPath+" (written by an earlier Astro CLI preview, removed")

	files[staleHelperPath] = "# our own\n"
	dir, res, _ = initWith(t, files)
	assert.FileExists(t, filepath.Join(dir, staleHelperPath))
	assert.NotContains(t, strings.Join(res.Deleted, "\n"), staleHelperPath)
}

// The manifest reference shows the file a conversion writes, byte for byte, so
// a change to the recipe has to change the page.
func TestManifestReferenceShowsTheRewrittenDockerfile(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "manifest-reference.md"))
	require.NoError(t, err)
	want := rewrittenDockerfile("", "ENV TZ=UTC\n")
	// A Windows checkout may give the page CRLF; the template is the same either way.
	assert.Contains(t, strings.ReplaceAll(string(body), "\r\n", "\n"), "```dockerfile\n"+want+"```\n")
}
