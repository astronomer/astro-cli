package airflowrt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerfileStages(t *testing.T) {
	stages := DockerfileStages([]byte("ARG V=1\n" +
		"FROM python:3.12-slim AS builder\n" +
		"RUN true\n" +
		"from\t--platform=linux/amd64 quay.io/astronomer/astro-runtime:12.6.0 as App\n" +
		"FROM registry.example/runtime:3.1-12@sha256:abc\n"))
	require.Len(t, stages, 3)
	assert.Equal(t, DockerfileStage{Line: 1, Ref: "python:3.12-slim", Image: "python", Tag: "3.12-slim", Alias: "builder"}, stages[0])
	assert.Equal(t, DockerfileStage{
		Line: 3, Flags: []string{"--platform=linux/amd64"}, Ref: "quay.io/astronomer/astro-runtime:12.6.0",
		Image: "quay.io/astronomer/astro-runtime", Tag: "12.6.0", Alias: "App",
	}, stages[1])
	assert.True(t, stages[2].Digest)
	assert.Equal(t, "3.1-12", stages[2].Tag)
}

// Every Astro Runtime tag carries the ONBUILD triggers except a -base one,
// whatever its flavor.
func TestOnBuildRuntime(t *testing.T) {
	for ref, want := range map[string]bool{
		"quay.io/astronomer/astro-runtime:12.6.0":                  true,
		"quay.io/astronomer/astro-runtime:13.12.0-slim":            true,
		"astrocrpublic.azurecr.io/runtime:3.1-12-python-3.12":      true,
		"mirror.example/runtime:3.1-12":                            true,
		"quay.io/astronomer/astro-runtime:12.6.0-base":             false,
		"quay.io/astronomer/astro-runtime:13.12.0-slim-base":       false,
		"astrocrpublic.azurecr.io/runtime:3.1-12-python-3.12-base": false,
		"python:3.12-slim": false,
		"quay.io/astronomer/astro-runtime@sha256:abc": false,
		"${BASE}": false,
	} {
		stages := DockerfileStages([]byte("FROM " + ref + "\n"))
		require.Len(t, stages, 1, ref)
		assert.Equal(t, want, stages[0].OnBuildRuntime(), ref)
	}
}

func TestBaseOf(t *testing.T) {
	assert.Equal(t, "quay.io/astronomer/astro-runtime:12.6.0-python-3.11-base", BaseOf("quay.io/astronomer/astro-runtime:12.6.0-python-3.11"))
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.3-base", BaseOf("astrocrpublic.azurecr.io/runtime:3.3"))
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.3-base", BaseOf("astrocrpublic.azurecr.io/runtime:3.3-base"))
}

// The install base is the recipe alone, reading the lists from its own
// context, and mounts the netrc secret only when asked; a generated build is
// the same recipe, then the project's files.
func TestInstallBaseDockerfile(t *testing.T) {
	df := InstallBaseDockerfile("img:1-base", false)
	assert.True(t, strings.HasPrefix(df, "FROM img:1-base\nCOPY packages.txt .\n"), df)
	assert.Contains(t, df, "COPY requirements.txt uv-lock-constraints.txt routed-requirements.txt routed-links.html ./\n")
	assert.NotContains(t, df, "--mount")
	assert.NotContains(t, df, ProjectCopy, "the install base carries no project files")
	assert.True(t, strings.HasSuffix(df, "USER astro\n"), df)
	assert.Contains(t, InstallBaseDockerfile("img:1-base", true), "RUN --mount=type=secret,id=netrc,target=/root/.netrc,required=false "+
		"if [ -s routed-requirements.txt ]; then pip install --no-cache-dir --root-user-action=ignore --no-index --no-deps --find-links routed-links.html -r routed-requirements.txt; fi && "+
		"if [ -x /usr/local/bin/install-python-dependencies ]")

	gen := GeneratedDockerfile("img:1-base", false)
	assert.Equal(t, df+ProjectCopy+"\n", gen)
}

func TestUsesInstallBase(t *testing.T) {
	assert.True(t, UsesInstallBase([]byte("ARG ASTRO_BASE\nFROM ${ASTRO_BASE}\nRUN true\n")))
	assert.True(t, UsesInstallBase([]byte("ARG ASTRO_BASE\nFROM python:3.12 AS b\nFROM --platform=linux/amd64 $ASTRO_BASE AS app\n")))
	assert.False(t, UsesInstallBase([]byte("ARG ASTRO_BASE\nFROM ${ASTRO_BASE} AS b\nFROM python:3.12\n")), "only the final stage counts")
	assert.False(t, UsesInstallBase([]byte("FROM quay.io/astronomer/astro-runtime:12.6.0\n")))
	assert.False(t, UsesInstallBase([]byte("ARG BASE\nFROM ${BASE}\n")))
}

func TestWithRegistry(t *testing.T) {
	assert.Equal(t, "registry.example.com/astronomer/runtime:3.3-8-python-3.13",
		WithRegistry("astrocrpublic.azurecr.io/runtime:3.3-8-python-3.13", "registry.example.com/astronomer/"))
	assert.Equal(t, "mirror/astro-runtime:12.6.0", WithRegistry("quay.io/astronomer/astro-runtime:12.6.0", "mirror"))
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.3", WithRegistry("astrocrpublic.azurecr.io/runtime:3.3", ""))
}

func TestRegistryOf(t *testing.T) {
	for image, want := range map[string]string{
		"astrocrpublic.azurecr.io/runtime":               "",
		"quay.io/astronomer/astro-runtime":               "",
		"registry.corp.example/astronomer/astro-runtime": "registry.corp.example/astronomer",
		"localhost:5000/runtime":                         "localhost:5000",
	} {
		got, ok := RegistryOf(image)
		assert.True(t, ok, image)
		assert.Equal(t, want, got, image)
	}
	_, ok := RegistryOf("python")
	assert.False(t, ok)
}
