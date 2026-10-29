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
	// python_version == 'X.Y' as python_full_version == 'X.Y.*'. (This branch's
	// own version 3, checking the image's Airflow against uv.lock's by release,
	// was 68adef75...; it never reached a release, and 4 carries both.)
	3: "72b26579905db375023b6b1e60149e277de2aecb385fdf5822bb23019a785f64",
	// Both: the image's Airflow checked against uv.lock's by release, and the
	// lock's constraints compared by meaning.
	4: "1b51bc76862c3eac04d849400c1fa85a6a8e0de18d0b16034af3b2e68cd28538",
	// 4, and the astro-runtime dependency group not counted as the project
	// naming a base image package.
	5: "9e63e4d8a11522988c9bc9639325f7ed479dc4429cd3dd0646c56aaeffe2913e",
	// The Airflow comparison runs only on a lock that is not stale, reads the
	// pins' build from the [tool.uv] pins header alone, and names astro init.
	6: "80b93403892ef730f62f9a4a698e03c0b46b5f653c112f77ec0fe7d480853253",
	// Reads the build uv.lock was seeded from out of the record astro writes
	// first in the [dependency-groups] astro-runtime group, and names astro
	// init as what seeds the lock again.
	7: "f75055dd939d1db94bac3940afb66b3a8e0a65ae13c38138f34098b7fab96b68",
	// Reads the build the project builds on out of the runtime record astro
	// writes in [tool.uv] constraint-dependencies, and counts every dependency
	// group as the project's own naming.
	8: "bc6b44ed6a0cfae918e704a407a48715c63a6e269bc57379ba1c682db4c4e440",
	// A stale lock's warning says to run uv lock, which is what relocks.
	9: "eb4ca08c703fc89c58dd398bac0e540edaf0dde018963818e342c3a4d95f1be3",
	// Reads markers, requirements and specifiers with packaging (or pip's
	// vendored copy) in place of its own evaluator; shorter prose. (#2394's
	// own version 4, 373f3dbc…, is the same helper without the Airflow check.)
	10: "e63c29660b49a9107092d9e97205533280b40c2a77701bbb13d79a7a90185706",
	// The Airflow-mismatch warning says only to run astro init, which relocks.
	11: "a698faee032795b637cbc83bad745149818a80acf5bc141405364cb4e7e13081",
	// Hands the resolving to uv (uv lock --check, uv lock, uv export --format
	// pylock.toml), copied into its stage, and keeps the image's policy and the
	// Airflow check. (#2394's own version 5, 7fac3db1…, is the same helper
	// without the Airflow check.)
	12: "de514ad1d7acdb7d75bad64959693a419526c70fadc524af2d70d4058a2eaceb",
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
