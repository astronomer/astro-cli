package airflowrt

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperContentAtVersion pins the helper's content to its version stamp. A
// change to install-project.py that keeps the stamp fails here: bump the
// "# astro-install-helper:" line and add the new hash, so astro init knows a
// project's copy is older and refreshes it.
var helperContentAtVersion = map[int]string{
	1: "b21a2237f2142b9bb010dd1dbe211ab9a9ce3f5c2f567e64a668315bd0ca326d",
	// Writes its outputs with LF line endings on every host.
	2: "e2c1f44df4ed6b3cc93e73e45473fc4b9549322b876962fe29fa62f9e36846a7",
	// Compares a lock's recorded constraints by name and specifier, and reads
	// python_version == 'X.Y' as python_full_version == 'X.Y.*'.
	3: "72b26579905db375023b6b1e60149e277de2aecb385fdf5822bb23019a785f64",
	// Reads markers, requirements and specifiers with packaging (or pip's
	// vendored copy) in place of its own evaluator; shorter prose.
	4: "373f3dbcaba5899d23c8335cbff5714c37dfd190e027b404ce83c2426e3f5952",
	// Hands the resolving to uv (uv lock --check, uv lock, uv export --format
	// pylock.toml), copied into its stage; keeps only the image's policy.
	5: "21f166c41d1c858c6d75fe0e083b8219b4ff1b0863d21f042ae72d42dd4629a5",
}

func TestInstallHelperVersionMovesWithItsContent(t *testing.T) {
	v := InstallHelperVersion()
	require.Positive(t, v, "the helper carries a version stamp")
	sum := sha256.Sum256(InstallHelper())
	assert.Equal(t, helperContentAtVersion[v], hex.EncodeToString(sum[:]),
		"install-project.py changed without its version stamp moving: bump it and pin the new hash")
}

func TestHelperVersion(t *testing.T) {
	v, ok := HelperVersion([]byte("# Written by the Astro CLI.\n# astro-install-helper: 7\r\n#\n"))
	assert.True(t, ok)
	assert.Equal(t, 7, v)
	_, ok = HelperVersion([]byte("# Written by the Astro CLI.\nprint('hi')\n"))
	assert.False(t, ok, "an unstamped copy has no version")
}

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

// The install reads the helper from the path it is written to.
func TestInstallDockerfileRunsTheHelperAtItsPath(t *testing.T) {
	df := InstallDockerfile("img:1-base")
	assert.Contains(t, df, " "+InstallHelperPath+" ", "the stage copies the helper in")
	assert.Contains(t, df, " python /tmp/astro-project/install-project.py ")
	assert.Contains(t, df, "FROM img:1-base AS "+InstallStage+"\n")
	assert.True(t, strings.HasPrefix(df, "ARG "+UVImageArg+"="+UVImage+"\nFROM ${"+UVImageArg+"} AS "+UVStage+"\n"),
		"the uv stage comes first, its build argument ahead of every FROM")
	assert.Contains(t, df, "COPY --from="+UVStage+" /uv /opt/astro-uv/uv\n")
	assert.Contains(t, InstallDockerfileWith("img:1-base", InstallOptions{UVImage: "mirror.example/uv:1"}), "ARG "+UVImageArg+"=mirror.example/uv:1\n")
	assert.NotEmpty(t, InstallHelper())
}
