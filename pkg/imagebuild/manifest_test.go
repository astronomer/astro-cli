package imagebuild

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// declaredProject writes a project with a Dockerfile at docker/Dockerfile and
// returns its root.
func declaredProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docker"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker", "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN --mount=type=secret,id=tok true\n"), 0o600))
	return dir
}

func TestForManifestDeclaredDockerfileIsTheBuild(t *testing.T) {
	dir := declaredProject(t)
	// An Airflow 2 pin, which RuntimeImage refuses: resolving a base for a
	// declared Dockerfile would fail this, so passing proves none is resolved.
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     dir,
		AirflowVersion: "2.9",
		Dockerfile:     "docker/Dockerfile",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "docker", "Dockerfile"), req.Dockerfile)
	assert.Equal(t, dir, req.Context, "a declared Dockerfile builds with the project as its context")
	assert.Empty(t, req.BaseImage, "a declared Dockerfile names its own FROM")
	assert.True(t, req.FromDeclaredDockerfile())
}

func TestForManifestGeneratesFromTheAirflowPin(t *testing.T) {
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     t.TempDir(),
		AirflowVersion: "3.1",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.1", req.BaseImage)
	assert.Empty(t, req.Dockerfile)
	assert.Empty(t, req.Context, "a generated build writes its own context under WorkDir")
	assert.False(t, req.FromDeclaredDockerfile())
}

// A [tool.astro] runtime names the build the generated image starts FROM, in
// place of the series tag.
func TestForManifestBuildsFromTheRuntimeBuild(t *testing.T) {
	for _, pin := range []string{"3.3", "3.3.1", "3"} {
		req, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: pin, Runtime: "3.3-8"}, nil)
		require.NoError(t, err, pin)
		assert.Equal(t, RuntimeImageRepo+":3.3-8", req.BaseImage, pin)
	}
	// An Airflow 2 build is a runtime version, under its own repository, and
	// naming it needs no catalog.
	req, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "2.11", Runtime: "13.11.0"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:13.11.0", req.BaseImage)
}

// The build the lock was made for is the base for a packaged and a local
// image when the manifest names no runtime, so the image is the build the
// lock matches rather than the newest of the series. [tool.astro] runtime
// still wins, a declared Dockerfile resolves no base at all, and with no
// lock build the series tag stays the base.
func TestTheLocksBuildIsTheBaseWhenNothingElseNamesOne(t *testing.T) {
	declared := declaredProject(t)
	for _, tt := range []struct {
		name string
		m    ManifestBuild
		want string
	}{
		{"lock build", ManifestBuild{AirflowVersion: "3.3", LockedRuntime: "3.3-8"}, RuntimeImageRepo + ":3.3-8"},
		{"runtime wins", ManifestBuild{AirflowVersion: "3.3", Runtime: "3.3-7", LockedRuntime: "3.3-8"}, RuntimeImageRepo + ":3.3-7"},
		{"no lock build", ManifestBuild{AirflowVersion: "3.3"}, RuntimeImageRepo + ":3.3"},
		{"airflow 2 lock build", ManifestBuild{AirflowVersion: "2.10", LockedRuntime: "12.6.0"}, "quay.io/astronomer/astro-runtime:12.6.0"},
		{"declared dockerfile", ManifestBuild{ProjectDir: declared, AirflowVersion: "3.3", LockedRuntime: "3.3-8", Dockerfile: "docker/Dockerfile"}, ""},
	} {
		if tt.m.ProjectDir == "" {
			tt.m.ProjectDir = t.TempDir()
		}
		pkg, err := ForManifest(tt.m, nil)
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, pkg.BaseImage, "%s: package", tt.name)

		local, err := ForLocalManifest(context.Background(), tt.m, noCatalog)
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, local.BaseImage, "%s: local", tt.name)
	}
}

// lockedCatalog lists an older build of Airflow 3.3 that ships only Python
// 3.12, and the newest, which ships 3.12 and 3.13.
const lockedCatalog = `{"runtimeVersionsV3": {
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}},
    "3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.12"}}
  }}`

// A requires-python edited since the lock was seeded, admitting none of the
// Pythons the lock's build ships, builds on the newest build of the series as
// it would with no lock, rather than failing, and the notices say why. With a
// requires-python the lock's build admits, the notice is that a newer build is
// available.
func TestTheLocksBuildGivesWayToRequiresPython(t *testing.T) {
	c, err := runtimeversions.Parse([]byte(lockedCatalog))
	require.NoError(t, err)
	catalog := func() *runtimeversions.Catalog { return c }

	m := ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.3", LockedRuntime: "3.3-8", RequiresPython: "==3.13.*"}
	req, err := ForManifest(m, catalog)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3-10-python-3.13", req.BaseImage)
	notices := LockedRuntimeNotices(m, c)
	require.Len(t, notices, 1)
	assert.Contains(t, notices[0], "admits none of the Pythons Astro Runtime 3.3-8 ships")

	m.RequiresPython = ">=3.12"
	req, err = ForManifest(m, catalog)
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.3-8", req.BaseImage)
	assert.Equal(t, []string{"Astro Runtime 3.3-10 is available; this project builds on 3.3-8. Run astro init to move to it"},
		LockedRuntimeNotices(m, c))

	m.LockedRuntime = "3.3-10"
	assert.Empty(t, LockedRuntimeNotices(m, c), "the newest build has nothing to move to")

	yanked, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
    "3.3-8": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}},
    "3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "yanked": true, "yankedReason": "a scheduler regression", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}}
  }}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"Astro Runtime 3.3-10, which this project builds on, is yanked (a scheduler regression). Run astro init to move to 3.3-8"},
		LockedRuntimeNotices(m, yanked), "a yanked lock build says so, though no newer build exists")
	m.Runtime = "3.3-8"
	assert.Empty(t, LockedRuntimeNotices(m, c), "a runtime the user named is theirs")
}

// With no build named, an Airflow 2 pin is looked up in the catalog, so one
// that cannot be read refuses the build and says how to name one instead.
func TestForManifestRefusesAPinWithNoRuntimeImage(t *testing.T) {
	_, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "2.9"}, nil)
	assert.ErrorContains(t, err, "[tool.astro] runtime")
	for _, pin := range []string{"", "1.10", "3"} {
		_, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: pin}, nil)
		assert.Error(t, err, "pin %q", pin)
	}
}

func TestForManifestCarriesDependenciesAndPackages(t *testing.T) {
	deps := []string{"apache-airflow==3.1.0", "pandas>=2"}
	pkgs := []string{"libpq-dev", "gcc"}
	for name, m := range map[string]ManifestBuild{
		"generated": {ProjectDir: t.TempDir(), AirflowVersion: "3.1"},
		"declared":  {ProjectDir: declaredProject(t), Dockerfile: "docker/Dockerfile"},
	} {
		m.Dependencies, m.Packages = deps, pkgs
		req, err := ForManifest(m, nil)
		require.NoError(t, err, name)
		assert.Equal(t, deps, req.Dependencies, name)
		assert.Equal(t, pkgs, req.Packages, name)
	}
}

// The request ForManifest returns, handed to Build, installs the manifest's
// dependencies and packages over the pin's base.
func TestForManifestGeneratedRequestInstallsDependenciesAndPackages(t *testing.T) {
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     t.TempDir(),
		AirflowVersion: "3.1",
		Dependencies:   []string{"apache-airflow==3.1.0", "pandas>=2"},
		Packages:       []string{"libpq-dev"},
	}, nil)
	require.NoError(t, err)
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-deploy/p", "docker"

	cmd := &fakeCmd{}
	built, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astro-deploy/p", built)

	reqs, pkgs, _ := installLists(t, filepath.Join(req.WorkDir, buildContextDir))
	assert.Equal(t, "pandas>=2\n", reqs)
	assert.Equal(t, "libpq-dev\n", pkgs)
	df, err := os.ReadFile(filepath.Join(req.WorkDir, dockerfileName))
	require.NoError(t, err)
	assert.Equal(t, GeneratedDockerfile(RuntimeImageRepo+":3.1", false), string(df))
}

// A secret other than netrc reaches the build exactly when a Dockerfile is
// declared. Both sides are asserted against the fixed expectation rather than
// against each other, since Build's own gate reads FromDeclaredDockerfile and
// the two would move together if it were wrong.
func TestFromDeclaredDockerfileMatchesWhereSecretsReachTheBuild(t *testing.T) {
	cases := []struct {
		name     string
		m        ManifestBuild
		declared bool
	}{
		{"generated", ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.1", Dependencies: []string{"pandas"}}, false},
		{"declared", ManifestBuild{ProjectDir: declaredProject(t), Dockerfile: "docker/Dockerfile"}, true},
	}
	for _, tc := range cases {
		req, err := ForManifest(tc.m, nil)
		require.NoError(t, err, tc.name)
		req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-deploy/p", "docker"
		req.Secrets = []string{"id=tok,env=TOK"}

		cmd := &fakeCmd{}
		_, err = testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.declared, req.FromDeclaredDockerfile(), tc.name)
		assert.Equal(t, tc.declared, hasCall(cmd.calls, "--secret id=tok,env=TOK"), "%s: %v", tc.name, cmd.calls)
	}
}

// With nothing to install, BuildLocal still builds the generated Dockerfile
// at the requested platform, so the tag it returns names a single-platform
// image in the local store rather than the base's multi-platform index.
func TestBuildLocalBuildsEvenWithNothingToInstall(t *testing.T) {
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     t.TempDir(),
		AirflowVersion: "3.1",
		Dependencies:   []string{"apache-airflow==3.1.*"},
	}, nil)
	require.NoError(t, err)
	req.WorkDir, req.Tag, req.Bin, req.Platform = t.TempDir(), "astro-deploy/p", "docker", "linux/amd64"

	cmd := &fakeCmd{}
	built, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astro-deploy/p", built, "the built tag, never the base")
	require.Len(t, cmd.calls, 1, "one build and nothing else, got %v", cmd.calls)
	assert.Contains(t, cmd.calls[0], "docker build --tag astro-deploy/p")
	assert.Contains(t, cmd.calls[0], "--platform linux/amd64")
	assert.False(t, hasCall(cmd.calls, "docker pull"), "%v", cmd.calls)

	// The install steps COPY what the helper writes, so it writes every file,
	// and they install nothing.
	reqs, pkgs, constraints := installLists(t, filepath.Join(req.WorkDir, buildContextDir))
	assert.Empty(t, reqs)
	assert.Empty(t, pkgs)
	assert.Empty(t, constraints)
	df, err := os.ReadFile(filepath.Join(req.WorkDir, dockerfileName))
	require.NoError(t, err)
	assert.Equal(t, GeneratedDockerfile(RuntimeImageRepo+":3.1", false), string(df))
}

// Build keeps its fast path: local docker mode runs the base as-is at the host
// platform when there is nothing to install.
func TestBuildKeepsTheFastPathBuildLocalSkips(t *testing.T) {
	req, err := ForLocalManifest(context.Background(), ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.1"}, noCatalog)
	require.NoError(t, err)
	require.Empty(t, req.ProjectFiles, "local start runs the project mounted")
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-local/p", "docker"

	cmd := &fakeCmd{}
	built, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.BaseImage+"-base", built)
	assert.Empty(t, cmd.calls)
}

// A shipped image carries the project's files: a DAG importing include/ fails
// on Astro without them, since a deploy sends dags/ alone beside the image.
func TestForManifestBuildCarriesTheProject(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"include/agents/__init__.py":           "x = 1\n",
		"dags/blueprints.py":                   "import include.agents\n",
		"pyproject.toml":                       "[project]\ndependencies = ['pandas']\n",
		".astro/config.yaml":                   "committed\n",
		"Requirements.txt":                     "stale-pin==1\n", // any case: never the generated one
		".venv/lib/site.py":                    "never\n",
		".git/HEAD":                            "never\n",
		"include/__pycache__/a.pyc":            "never\n",
		".env":                                 "SECRET=never\n",
		".astro/standalone/airflow.db":         "never\n",
		"plugins/fix_local_executor_pickle.py": "never\n",
		"tests/test_dags.py":                   "ignored by the project\n",
		// Never shipped, at any depth: local env values and 1.x settings.
		"include/.env":                      "SECRET=never\n",
		".env.prod":                         "SECRET=never\n",
		"include/cfg/.env.local":            "SECRET=never\n",
		".envrc":                            "export SECRET=never\n",
		"airflow_settings.yaml":             "never\n",
		"include/airflow_settings.yaml":     "never\n",
		"airflow.db":                        "never\n",
		"airflow.cfg":                       "never\n",
		"logs/scheduler/latest.log":         "never\n",
		"include/logs/fixture.json":         "the project's own\n",
		"include/airflow.cfg":               "the project's own\n",
		"include/sample_env":                "the project's own\n",
		"include/environment/defaults.yaml": "the project's own\n",
		// The project's own rules apply, but cannot bring a built-in back.
		".dockerignore": "tests/\n!.env\n!**/.env\n!airflow_settings.yaml\n!airflow.cfg\n!logs/\n",
	})
	req, err := ForManifest(ManifestBuild{ProjectDir: project, AirflowVersion: "3.1", Dependencies: []string{"pandas"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, project, req.ProjectFiles)
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-deploy/p", "docker"
	// A file left over from an earlier build in the same WorkDir is not carried.
	stale := filepath.Join(req.WorkDir, buildContextDir, "include", "deleted.py")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, os.WriteFile(stale, nil, 0o600))

	_, err = testBuilder(&fakeCmd{}).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)

	ctxDir := filepath.Join(req.WorkDir, buildContextDir)
	assert.ElementsMatch(t, []string{
		"include/agents/__init__.py", "dags/blueprints.py", "pyproject.toml", ".astro/config.yaml",
		"include/logs/fixture.json", "include/airflow.cfg", "include/sample_env", "include/environment/defaults.yaml",
		".astro/install/install-project.py", ".astro/install/pyproject.toml",
	}, contextFiles(t, ctxDir))
	reqs, _, _ := installLists(t, ctxDir)
	assert.Equal(t, "pandas\n", reqs, "the generated requirements, not the project's")
}

// Symlinks are carried as what they point at: a link out of the project would
// dangle in the image, and Windows users mostly cannot create one. A link back
// up the tree is not followed forever, a dangling one is left out, and a
// project directory that is itself a symlink is walked.
func TestProjectFilesFollowSymlinks(t *testing.T) {
	outside := t.TempDir()
	writeTree(t, outside, map[string]string{"lib/shared.py": "shared = 1\n"})
	projectDir := t.TempDir()
	writeTree(t, projectDir, map[string]string{"include/own.py": "own = 1\n"})
	for link, target := range map[string]string{
		"include/shared":  filepath.Join(outside, "lib"),
		"include/loop":    filepath.Join(projectDir, "include"),
		"include/missing": filepath.Join(outside, "gone"),
	} {
		if err := os.Symlink(target, filepath.Join(projectDir, filepath.FromSlash(link))); err != nil {
			t.Skipf("symlinks unavailable here: %v", err)
		}
	}
	project := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.Symlink(projectDir, project))

	dst := t.TempDir()
	require.NoError(t, copyProjectFiles(project, dst))
	assert.ElementsMatch(t, []string{"include/own.py", "include/shared/shared.py"}, contextFiles(t, dst))
	info, err := os.Lstat(filepath.Join(dst, "include", "shared", "shared.py"))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "carried as a file, not a link")
}

// The digest a package tag includes moves with what the image carries, and
// only with that.
func TestProjectFilesDigest(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{"include/a.py": "a = 1\n", ".dockerignore": "scratch/\n"})
	first, err := ProjectFilesDigest(project)
	require.NoError(t, err)

	writeTree(t, project, map[string]string{"scratch/notes.txt": "ignored\n", ".env": "SECRET=x\n", ".venv/x": "y\n"})
	same, err := ProjectFilesDigest(project)
	require.NoError(t, err)
	assert.Equal(t, first, same, "nothing the image carries changed")

	writeTree(t, project, map[string]string{"include/a.py": "a = 2\n"})
	edited, err := ProjectFilesDigest(project)
	require.NoError(t, err)
	assert.NotEqual(t, first, edited)
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, body := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o600))
	}
}

// contextFiles lists the regular files under dir, slash-separated.
func contextFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		files = append(files, filepath.ToSlash(rel))
		return err
	}))
	return files
}

// A declared Dockerfile builds the project as its context already.
func TestForManifestDeclaredCarriesNoProjectFiles(t *testing.T) {
	req, err := ForManifest(ManifestBuild{ProjectDir: declaredProject(t), Dockerfile: "docker/Dockerfile"}, nil)
	require.NoError(t, err)
	assert.Empty(t, req.ProjectFiles)
}

func TestBuildLocalReturnsTheTagForEveryMode(t *testing.T) {
	for name, m := range map[string]ManifestBuild{
		"generated with deps": {ProjectDir: t.TempDir(), AirflowVersion: "3.1", Dependencies: []string{"pandas"}},
		"declared":            {ProjectDir: declaredProject(t), Dockerfile: "docker/Dockerfile"},
	} {
		req, err := ForManifest(m, nil)
		require.NoError(t, err, name)
		req.WorkDir, req.Tag, req.Bin, req.Platform = t.TempDir(), "astro-deploy/p", "docker", "linux/amd64"

		cmd := &fakeCmd{}
		built, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
		require.NoError(t, err, name)
		assert.Equal(t, "astro-deploy/p", built, name)
		assert.False(t, hasCall(cmd.calls, "docker pull"), "%s: %v", name, cmd.calls)
	}
}

func TestBuildLocalNamesAFailedBuild(t *testing.T) {
	req, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.1"}, nil)
	require.NoError(t, err)
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-deploy/p", "docker"

	boom := errors.New("registry unreachable")
	cmd := &fakeCmd{run: func(string, rt.Stdio) error { return boom }}
	_, err = testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "see the build output above")
}
