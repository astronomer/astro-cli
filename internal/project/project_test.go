package project

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

const windowsOS = "windows"

func writeMarker(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, Marker), []byte("[project]\nname = \"demo\"\n"), 0o600))
}

func TestDiscoverWalksUpFromNestedDir(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "my-project")
	nested := filepath.Join(proj, "dags", "sub")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	writeMarker(t, proj)

	p, err := Discover(nested)
	require.NoError(t, err)
	assert.Equal(t, proj, p.Dir)
	assert.Equal(t, "my-project.localhost", p.Hostname)
	assert.False(t, p.IsWorktree)
	assert.Len(t, p.ID, 64)
}

func TestDiscoverFindsMarkerInStartDir(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	writeMarker(t, proj)

	p, err := Discover(proj)
	require.NoError(t, err)
	assert.Equal(t, proj, p.Dir)
}

func TestDiscoverPrefersNearestMarker(t *testing.T) {
	outer := filepath.Join(t.TempDir(), "outer")
	inner := filepath.Join(outer, "inner")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	writeMarker(t, outer)
	writeMarker(t, inner)

	p, err := Discover(inner)
	require.NoError(t, err)
	assert.Equal(t, inner, p.Dir)
}

func TestDiscoverNotFound(t *testing.T) {
	dir := t.TempDir()

	p, err := Discover(dir)
	assert.Nil(t, p)
	var nf *NotFoundError
	require.ErrorAs(t, err, &nf)
	assert.Contains(t, nf.Error(), Marker)
}

func TestDiscoverIgnoresMarkerDirectory(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	nested := filepath.Join(proj, "sub")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	writeMarker(t, proj)
	// A directory named pyproject.toml is not a manifest.
	require.NoError(t, os.MkdirAll(filepath.Join(nested, Marker), 0o755))

	p, err := Discover(nested)
	require.NoError(t, err)
	assert.Equal(t, proj, p.Dir)
}

func TestIDSymlinkedPathsHashIdentically(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("symlinks need extra privileges on windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "real-project")
	require.NoError(t, os.MkdirAll(target, 0o755))
	writeMarker(t, target)
	link := filepath.Join(root, "link-project")
	require.NoError(t, os.Symlink(target, link))

	realID, err := ID(target)
	require.NoError(t, err)
	linkID, err := ID(link)
	require.NoError(t, err)
	assert.Equal(t, realID, linkID)

	p, err := Discover(link)
	require.NoError(t, err)
	assert.Equal(t, link, p.Dir)
	assert.Equal(t, realID, p.ID)
}

func TestIDDiffersPerDirectory(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	require.NoError(t, os.MkdirAll(a, 0o755))
	require.NoError(t, os.MkdirAll(b, 0o755))

	aID, err := ID(a)
	require.NoError(t, err)
	bID, err := ID(b)
	require.NoError(t, err)
	assert.NotEqual(t, aID, bID)
}

func TestDiscoverDetectsWorktree(t *testing.T) {
	root := t.TempDir()
	// Layout of a linked worktree: the main repo holds
	// .git/worktrees/<name>, and the worktree's .git is a file pointing
	// there.
	repo := filepath.Join(root, "main-repo")
	gitdir := filepath.Join(repo, ".git", "worktrees", "feature-x")
	require.NoError(t, os.MkdirAll(gitdir, 0o755))
	wt := filepath.Join(root, "feature-x")
	require.NoError(t, os.MkdirAll(wt, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o600))
	writeMarker(t, wt)

	p, err := Discover(wt)
	require.NoError(t, err)
	assert.True(t, p.IsWorktree)
	assert.Equal(t, "feature-x.main-repo.localhost", p.Hostname)

	wtID, err := ID(wt)
	require.NoError(t, err)
	assert.Equal(t, wtID, p.ID)
	repoID, err := ID(repo)
	require.NoError(t, err)
	assert.NotEqual(t, repoID, p.ID)
}

func TestDiscoverRegularRepoIsNotWorktree(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(filepath.Join(proj, ".git"), 0o755))
	writeMarker(t, proj)

	p, err := Discover(proj)
	require.NoError(t, err)
	assert.False(t, p.IsWorktree)
	assert.Equal(t, "proj.localhost", p.Hostname)
}

// validLabel matches a single DNS label: lowercase alphanumeric and hyphens,
// no leading or trailing hyphen.
var validLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func TestNewFallsBackToIDHostnameForUnusableName(t *testing.T) {
	// A directory name that sanitizes to an empty DNS label — all non-ASCII,
	// or all punctuation — must not break the project. Identity is the path
	// hash, so the hostname falls back to an ID-derived label.
	root := t.TempDir()
	dirs := map[string]string{
		"nonascii":    "日本語",
		"punctuation": "...",
		"normal":      "my-project",
	}
	hostnames := map[string]string{}
	for key, dirName := range dirs {
		proj := filepath.Join(root, key, dirName)
		require.NoError(t, os.MkdirAll(proj, 0o755))
		writeMarker(t, proj)

		p, err := Discover(proj)
		require.NoError(t, err)
		hostnames[key] = p.Hostname

		label, ok := strings.CutSuffix(p.Hostname, ".localhost")
		require.True(t, ok, "hostname %q should end in .localhost", p.Hostname)
		assert.Regexp(t, validLabel, label, "hostname label must be a valid DNS label")

		if key == "normal" {
			// A usable name still derives from the name — no identity churn.
			assert.Equal(t, "my-project.localhost", p.Hostname)
		} else {
			// Unusable names fall back to astro-<first 8 hex of ID>.localhost.
			assert.Equal(t, "astro-"+p.ID[:8]+".localhost", p.Hostname)
		}
	}

	// The two odd projects must not collide with each other or the normal one.
	assert.NotEqual(t, hostnames["nonascii"], hostnames["punctuation"])
	assert.NotEqual(t, hostnames["nonascii"], hostnames["normal"])
	assert.NotEqual(t, hostnames["punctuation"], hostnames["normal"])
}

func TestIDMissingDirectory(t *testing.T) {
	_, err := ID(filepath.Join(t.TempDir(), "gone"))
	assert.Error(t, err)
}

func TestNotFoundErrorMentionsStart(t *testing.T) {
	dir := t.TempDir()
	_, err := Discover(dir)
	var nf *NotFoundError
	require.True(t, errors.As(err, &nf))
	assert.Contains(t, nf.Start, filepath.Base(dir))
}

func TestNotFoundErrorPointsAtInit(t *testing.T) {
	_, err := Discover(t.TempDir())
	var nf *NotFoundError
	require.ErrorAs(t, err, &nf)
	assert.Empty(t, nf.V1Dir)
	assert.Contains(t, err.Error(), "`astro init`")
	assert.NotContains(t, err.Error(), "v1")
}

// A 1.x project has no marker, so Discover fails in it; the error says it is v1
// and that `astro init` upgrades it, from the project root or below it.
func TestDiscoverInV1ProjectNamesIt(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM x\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, ".astro"), 0o700))
	dags := filepath.Join(root, "dags")
	require.NoError(t, os.Mkdir(dags, 0o700))

	t.Run("at the root", func(t *testing.T) {
		_, err := Discover(root)
		var nf *NotFoundError
		require.ErrorAs(t, err, &nf)
		assert.Equal(t, root, nf.V1Dir)
		assert.Contains(t, err.Error(), "this directory holds a project made by Astro CLI 1.x")
		assert.Contains(t, err.Error(), "Run `astro init` here")
	})
	t.Run("below the root", func(t *testing.T) {
		_, err := Discover(dags)
		var nf *NotFoundError
		require.ErrorAs(t, err, &nf)
		assert.Equal(t, root, nf.V1Dir)
		assert.Contains(t, err.Error(), root+" holds a project made by Astro CLI 1.x")
		assert.Contains(t, err.Error(), "Run `astro init` in "+root)
	})
}

func TestLoadError(t *testing.T) {
	other := errors.New("other")
	assert.Same(t, other, LoadError(t.TempDir(), t.TempDir(), other))

	t.Run("a tools-only pyproject", func(t *testing.T) {
		dir := t.TempDir()
		err := LoadError(dir, dir, manifest.ErrNoAstroSection)
		var ns *NoAstroSectionError
		require.ErrorAs(t, err, &ns)
		assert.False(t, ns.V1)
		require.ErrorIs(t, err, manifest.ErrNoAstroSection)
		assert.Contains(t, err.Error(), "no [tool.astro] section")
		assert.Contains(t, err.Error(), "`astro init`")
	})
	t.Run("a tools-only pyproject in a 1.x project", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600))
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".astro"), 0o700))
		writeManifest(t, dir, toolsOnlyPyproject)
		err := LoadError(dir, dir, manifest.ErrNoAstroSection)
		var ns *NoAstroSectionError
		require.ErrorAs(t, err, &ns)
		assert.True(t, ns.V1)
		require.ErrorIs(t, err, manifest.ErrNoAstroSection)
		assert.Contains(t, err.Error(), "project made by Astro CLI 1.x")
		assert.Contains(t, err.Error(), "Run `astro init` here")

		// From below the root, `astro init` "here" would scaffold a second
		// project inside the v1 one, so the root is named instead.
		dags := filepath.Join(dir, "dags")
		require.NoError(t, os.Mkdir(dags, 0o700))
		err = LoadError(dags, dir, manifest.ErrNoAstroSection)
		assert.Contains(t, err.Error(), "Run `astro init` in "+dir)
		assert.NotContains(t, err.Error(), "here")
	})
}

func TestIsV1(t *testing.T) {
	writeDockerfile := func(t *testing.T, dir string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600))
	}
	writeAstroDir := func(t *testing.T, dir string) {
		t.Helper()
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".astro"), 0o700))
	}

	t.Run("Dockerfile and .astro is v1", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		writeAstroDir(t, dir)
		assert.True(t, IsV1(dir))
	})
	t.Run("Dockerfile alone is not v1", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		assert.False(t, IsV1(dir))
	})
	t.Run("pyproject dir is not v1", func(t *testing.T) {
		dir := t.TempDir()
		writeMarker(t, dir)
		assert.False(t, IsV1(dir))
	})
	t.Run("empty dir is not v1", func(t *testing.T) {
		assert.False(t, IsV1(t.TempDir()))
	})
	t.Run("a pyproject that only configures tools leaves a v1 layout v1", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		writeAstroDir(t, dir)
		writeManifest(t, dir, toolsOnlyPyproject)
		assert.True(t, IsV1(dir))
	})
	t.Run("a v2 manifest beside a v1 layout is not v1", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		writeAstroDir(t, dir)
		writeManifest(t, dir, v2Manifest)
		assert.False(t, IsV1(dir))
	})
	t.Run("an unparseable pyproject beside a v1 layout is not v1", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		writeAstroDir(t, dir)
		writeManifest(t, dir, "this is not : valid = toml [[[\n")
		assert.False(t, IsV1(dir))
	})
}

const v2Manifest = `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
`

const toolsOnlyPyproject = `[tool.ruff]
line-length = 120

[tool.sqlfluff.core]
dialect = "snowflake"

[tool.pytest.ini_options]
testpaths = ["tests"]
`

func writeManifest(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, Marker), []byte(content), 0o600))
}

func TestHasManifest(t *testing.T) {
	t.Run("valid manifest", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, v2Manifest)
		assert.True(t, HasManifest(dir))
	})
	t.Run("valid manifest beside a Dockerfile is still v2", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, v2Manifest)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
		assert.True(t, HasManifest(dir))
	})
	t.Run("missing pyproject", func(t *testing.T) {
		assert.False(t, HasManifest(t.TempDir()))
	})
	t.Run("pyproject without tool.astro", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, "[project]\nname = \"demo\"\n")
		assert.False(t, HasManifest(dir))
	})
	t.Run("pyproject that only configures tools", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, toolsOnlyPyproject)
		assert.False(t, HasManifest(dir))
	})
	t.Run("tool.astro present but invalid still counts as v2", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, "[project]\nname = \"demo\"\n\n[tool.astro]\n")
		assert.True(t, HasManifest(dir))
	})
	t.Run("unparseable pyproject counts as v2", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, "this is not : valid = toml [[[\n")
		assert.True(t, HasManifest(dir))
	})
}
