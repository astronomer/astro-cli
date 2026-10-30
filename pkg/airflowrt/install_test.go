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

// A project's own Dockerfile reads the lists from the named context, and
// mounts the netrc secret whether or not the build passes one; a generated
// build reads them from its own context, and mounts it only when asked.
func TestInstallDockerfileReadsTheListsFromTheNamedContext(t *testing.T) {
	df := InstallDockerfile("img:1-base")
	assert.True(t, strings.HasPrefix(df, "FROM img:1-base\nCOPY --from="+InstallContext+" packages.txt .\n"), df)
	assert.Contains(t, df, "COPY --from="+InstallContext+" requirements.txt uv-lock-constraints.txt ./\n")
	assert.Contains(t, df, "RUN --mount=type=secret,id=netrc,target=/root/.netrc,required=false if [ -x /usr/local/bin/install-python-dependencies ]")
	assert.NotContains(t, df, "ARG ", "no build argument, no helper stage")
	assert.Equal(t, 1, strings.Count(df, "FROM "))
	assert.Contains(t, InstallDockerfileWith("img:1-base", InstallOptions{ListsFrom: InstallContext, Alias: "app"}), "FROM img:1-base AS app\n")

	gen := InstallDockerfileWith("img:1-base", InstallOptions{})
	assert.Contains(t, gen, "\nCOPY packages.txt .\n")
	assert.NotContains(t, gen, "--mount")
	assert.Contains(t, InstallDockerfileWith("img:1-base", InstallOptions{MountNetrc: true}), "RUN --mount=type=secret,id=netrc")
}

func TestReadsInstallContext(t *testing.T) {
	assert.True(t, ReadsInstallContext([]byte(InstallDockerfile("img:1-base"))))
	assert.True(t, ReadsInstallContext([]byte("FROM x\nCOPY --from=\"astro-install\" requirements.txt .\n")))
	assert.False(t, ReadsInstallContext([]byte(InstallDockerfileWith("img:1-base", InstallOptions{}))), "a generated build reads its own context")
	assert.False(t, ReadsInstallContext([]byte("FROM x\n# COPY --from=astro-install packages.txt .\n")))
	assert.False(t, ReadsInstallContext([]byte("FROM x\nCOPY --from=astro-installer a .\n")))
}
