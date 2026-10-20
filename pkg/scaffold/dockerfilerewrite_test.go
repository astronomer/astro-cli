package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
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
			name:    "a stage named like the install's",
			body:    "FROM python:3.12-slim AS astro-install\nRUN true\nFROM quay.io/astronomer/astro-runtime:12.1.0\nENV A=b\n",
			blocker: "a stage named astro-install",
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
			name:    "a final stage named like the install's",
			body:    "FROM quay.io/astronomer/astro-runtime:12.1.0 AS astro-install\nENV A=b\n",
			blocker: "a stage named astro-install",
		},
		{
			name:    "a floating Airflow 2 tag",
			body:    "FROM quay.io/astronomer/astro-runtime:12\nENV A=b\n",
			blocker: "no -base tag is published for the floating Airflow 2 tag 12",
		},
		{name: "a file that names dev-requirements.txt, not requirements.txt", body: "FROM python:3.12-slim\nCOPY dev-requirements.txt /tmp/\n"},
		{name: "a comment naming requirements.txt", body: "FROM quay.io/astronomer/astro-runtime:12.1.0-base\n# requirements.txt is gone\n"},
		{name: "a final stage on an earlier -base stage", body: "FROM quay.io/astronomer/astro-runtime:12.1.0-base AS base\nFROM base\nRUN true\n"},
		{name: "a base from a build argument", body: "ARG BASE=quay.io/astronomer/astro-runtime:12.1.0\nFROM ${BASE}\nENV A=b\n"},
		{name: "a base already on -base", body: "FROM quay.io/astronomer/astro-runtime:12.1.0-base\nENV A=b\n"},
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

	from := "--platform=linux/amd64 astrocrpublic.azurecr.io/runtime:3.1-12-base"
	install := airflowrt.InstallDockerfileWith(from, airflowrt.InstallOptions{Alias: "app"})
	want := "ARG PY=3.12\nFROM python:3.12-slim AS builder\nRUN pip wheel --wheel-dir /wheels mylib\n\n" +
		installBlockComment + install + "\n" + ownStepsComment +
		"COPY --from=builder /wheels /wheels\nENV TZ=UTC\n"
	assert.Equal(t, want, string(rw.body))
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1-12", rw.from)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1-12-base", rw.base)
	assert.False(t, rw.mirror)

	// The install's helper stage is unnamed by the project and keeps the
	// install's own name; only the project's final stage carries "app".
	stages := airflowrt.DockerfileStages(rw.body)
	require.Len(t, stages, 3)
	assert.Equal(t, airflowrt.InstallStage, stages[1].Alias)
	assert.Equal(t, "app", stages[2].Alias)
	for _, st := range stages {
		assert.False(t, st.OnBuildRuntime(), "no stage is left on an ONBUILD tag: %s", st.Ref)
	}
}

// A conversion that rewrites says so in each list: the Dockerfile updated, the
// helper created, both 1.x lists retired.
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
	helper, err := os.ReadFile(filepath.Join(dir, airflowrt.InstallHelperPath))
	require.NoError(t, err)
	assert.Equal(t, airflowrt.InstallHelper(), helper)

	updated := strings.Join(res.Updated, "\n")
	assert.Contains(t, updated, "Dockerfile (now builds FROM quay.io/astronomer/astro-runtime:12.6.0-base")
	assert.Contains(t, strings.Join(res.Created, "\n"), airflowrt.InstallHelperPath)
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
	assert.NoFileExists(t, filepath.Join(dir, airflowrt.InstallHelperPath))
	assert.FileExists(t, filepath.Join(dir, "requirements.txt"))
	assert.FileExists(t, filepath.Join(dir, "packages.txt"), "the ONBUILD tag still reads it")

	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, "Dockerfile: kept FROM quay.io/astronomer/astro-runtime:12.6.0 rather than moved onto "+
		"quay.io/astronomer/astro-runtime:12.6.0-base, because requirements.txt has lines")
	assert.Contains(t, notes, "packages.txt: kept, because your Dockerfile's base image reads it")
}

// A mirror is rewritten onto its own -base tag, and the run says that tag has
// to be mirrored too.
func TestConversionOnAMirrorAdvisesMirroringTheBaseTag(t *testing.T) {
	_, res, _ := initWith(t, map[string]string{
		fileDockerfile: "FROM registry.corp.example/astronomer/astro-runtime:12.6.0\nENV A=b\n",
	})
	advisories := mirrorAdvisories(res)
	require.Len(t, advisories, 1)
	assert.Contains(t, advisories[0], "mirror registry.corp.example/astronomer/astro-runtime:12.6.0-base too")
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

// An ignore file that leaves out a file the rewritten build reads gains an
// exception for it, after the lines already there, so the build still sees it.
func TestConversionKeepsTheInstallInputsInTheContext(t *testing.T) {
	dir, res, _ := initWith(t, map[string]string{
		fileDockerfile:   "FROM quay.io/astronomer/astro-runtime:12.6.0\nENV A=b\n",
		fileDockerignore: "*.toml\n.astro\n.venv/\n**/.git\n",
	})

	pm, data, err := readDockerignore(dir, fileDockerignore)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "*.toml\n.astro\n"), "the file's own lines are kept: %q", data)
	for _, path := range installInputs {
		assert.False(t, excludes(pm, path, false), "%s is in the build's context", path)
	}
	assert.True(t, excludes(pm, ".astro/standalone", true), "the rest of .astro stays out")
	assert.Contains(t, strings.Join(res.Updated, "\n"), fileDockerignore+" (added the per-machine rules, and exceptions for the files the build reads)")
}

// The manifest reference shows the file a conversion writes, byte for byte, so
// a change to the recipe has to change the page.
func TestManifestReferenceShowsTheRewrittenDockerfile(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "manifest-reference.md"))
	require.NoError(t, err)
	want := rewrittenDockerfile("quay.io/astronomer/astro-runtime:12.6.0-base", "ENV TZ=UTC\n")
	// A Windows checkout may give the page CRLF; the template is the same either way.
	assert.Contains(t, strings.ReplaceAll(string(body), "\r\n", "\n"), "```dockerfile\n"+want+"```\n")
}

// installInputs are the manifest, its lock and the helper, by the names the
// helper's stage reads them under.
func TestInstallInputsAreWhatTheInstallStageReads(t *testing.T) {
	assert.Equal(t, []string{manifest.Marker, "uv.lock", airflowrt.InstallHelperPath}, installInputs)
	for _, path := range installInputs {
		assert.Contains(t, string(airflowrt.InstallHelper())+airflowrt.InstallDockerfile("x"), filepath.Base(path))
	}
}
