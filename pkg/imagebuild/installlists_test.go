package imagebuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// The lists, as the context holds them: requirements.txt names the lock's
// constraints with -c only when there are some, the routed links are an HTML
// page, and every file is there, empty or not, for the Dockerfile's COPY
// lines.
func TestInstallListsFiles(t *testing.T) {
	files := InstallLists{
		Requirements: []string{"pandas==2.2.3", "six @ https://example.com/six.whl#sha256=ab"},
		Constraints:  []string{"numpy==2.1.0 ; python_version >= '3.10'"},
		Packages:     []string{"libpq-dev"},
		Routed:       []string{"acme-lib==1.0 ; sys_platform == 'linux'"},
		RoutedLinks:  []string{"https://acme.example/acme_lib-1.0-py3-none-any.whl#sha256=ab", "https://acme.example/acme-lib-1.0.tar.gz?a=1&b=2#sha256=cd"},
	}.Files()
	assert.Equal(t, map[string][]byte{
		requirementsName:    []byte("-c uv-lock-constraints.txt\npandas==2.2.3\nsix @ https://example.com/six.whl#sha256=ab\n"),
		lockConstraintsName: []byte("numpy==2.1.0 ; python_version >= '3.10'\n"),
		packagesName:        []byte("libpq-dev\n"),
		routedName:          []byte("acme-lib==1.0 ; sys_platform == 'linux'\n"),
		routedLinksName: []byte("<!DOCTYPE html>\n<html><body>\n" +
			"<a href=\"https://acme.example/acme_lib-1.0-py3-none-any.whl#sha256=ab\">acme_lib-1.0-py3-none-any.whl</a>\n" +
			"<a href=\"https://acme.example/acme-lib-1.0.tar.gz?a=1&amp;b=2#sha256=cd\">acme-lib-1.0.tar.gz</a>\n" +
			"</body></html>\n"),
	}, files)

	empty := InstallLists{}.Files()
	assert.Len(t, empty, 5)
	for name, data := range empty {
		assert.Empty(t, data, name)
	}
	assert.True(t, InstallLists{Constraints: []string{"numpy==2.1.0"}}.Empty(), "constraints alone install nothing")
	assert.False(t, InstallLists{Routed: []string{"acme-lib==1.0"}}.Empty(), "a routed package installs")
}

// The digest follows the files, so a change to any list moves it.
func TestInstallListsDigest(t *testing.T) {
	a := InstallLists{Requirements: []string{"pandas==2.2.3"}}
	assert.Equal(t, a.Digest(), InstallLists{Requirements: []string{"pandas==2.2.3"}}.Digest())
	assert.NotEqual(t, a.Digest(), InstallLists{Requirements: []string{"pandas==2.2.2"}}.Digest())
	assert.NotEqual(t, a.Digest(), InstallLists{Requirements: []string{"pandas==2.2.3"}, Constraints: []string{"numpy==2.1.0"}}.Digest())
	assert.NotEqual(t, a.Digest(), InstallLists{Requirements: []string{"pandas==2.2.3"}, Packages: []string{"git"}}.Digest())
	assert.NotEqual(t, a.Digest(), InstallLists{Requirements: []string{"pandas==2.2.3"}, RoutedLinks: []string{"https://acme.example/x.whl#sha256=ab"}}.Digest())
}

// Without a lock to pin from, the dependencies install as written, without
// Airflow, after their find-links pages, and a git reference brings git.
func TestUnpinnedLists(t *testing.T) {
	l := UnpinnedLists([]string{"apache-airflow==3.1.*", "pandas>=2", "x @ git+https://example.com/x"},
		[]string{"https://pypi.acme.example/simple/x/"}, []string{"libpq-dev"})
	assert.Equal(t, []string{"--find-links https://pypi.acme.example/simple/x/", "pandas>=2", "x @ git+https://example.com/x"}, l.Requirements)
	assert.Empty(t, l.Constraints)
	assert.Equal(t, []string{"libpq-dev", "git"}, l.Packages)

	assert.Equal(t, []string{"git=1:2.39"}, WithGit([]string{"git=1:2.39"}), "git already listed")
	assert.Equal(t, []string{"pandas"}, UnpinnedLists([]string{"pandas"}, nil, nil).Requirements)
	assert.Empty(t, UnpinnedLists([]string{"pandas"}, nil, nil).Packages)
}

// A request carrying resolved lists installs those, not its own fields.
func TestBuildInstallsTheResolvedLists(t *testing.T) {
	req := testRequest(t)
	req.Dependencies, req.Packages = []string{"pandas"}, []string{"never"}
	req.Install = &InstallLists{Requirements: []string{"pandas==2.2.3"}, Constraints: []string{"numpy==2.1.0"}}

	_, err := testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	ctxDir := filepath.Join(req.WorkDir, buildContextDir)
	for name, want := range req.Install.Files() {
		got, err := os.ReadFile(filepath.Join(ctxDir, name))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), name)
	}
}

// Whether there is anything to install is the resolved lists' to say when
// there are some.
func TestBuildFastPathFollowsTheResolvedLists(t *testing.T) {
	for name, tc := range map[string]struct {
		install InstallLists
		builds  bool
	}{
		"a requirement":    {InstallLists{Requirements: []string{"pandas==2.2.3"}}, true},
		"an OS package":    {InstallLists{Packages: []string{"libpq-dev"}}, true},
		"only constraints": {InstallLists{Constraints: []string{"numpy==2.1.0"}}, false},
		"nothing, though the request lists pandas": {InstallLists{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := &fakeCmd{}
			req := testRequest(t)
			req.Dependencies = []string{"pandas"}
			req.Install = &tc.install
			image, err := testBuilder(cmd).Build(t.Context(), req, rt.Callbacks{})
			require.NoError(t, err)
			if tc.builds {
				assert.Equal(t, req.Tag, image)
				assert.NotEmpty(t, cmd.calls)
				return
			}
			assert.Equal(t, req.BaseImage+"-base", image)
			assert.Empty(t, cmd.calls)
		})
	}
}

// A project that keeps its files read-only ships them read-only into the
// context, and the build still writes its own lists beside them.
func TestBuildReplacesReadOnlyProjectFiles(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{"pyproject.toml": "[project]\nname = 'p'\n", "requirements.txt": "stale==1\n"})
	for _, name := range []string{"pyproject.toml", "requirements.txt"} {
		require.NoError(t, os.Chmod(filepath.Join(project, name), 0o444))
	}
	req := testRequest(t)
	req.ProjectFiles = project
	req.Install = &InstallLists{Requirements: []string{"pandas==2.2.3"}}

	_, err := testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, requirementsName))
	require.NoError(t, err)
	assert.Equal(t, "pandas==2.2.3\n", string(got))
}

// An older build left a find-links file in a project's .astro. It is taken
// out of the context, and never read.
func TestBuildDropsAFindLinksFileTheProjectCarries(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"pyproject.toml":                 "[project]\nname = 'p'\n",
		".astro/" + installFindLinksName: "https://evil.example/simple/pandas/\n",
	})
	req := testRequest(t)
	req.ProjectFiles = project
	req.Dependencies = []string{"pandas"}

	_, err := testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(req.WorkDir, buildContextDir, ".astro", installFindLinksName))
}

// A written context is the generated build, whole: the Dockerfile at its
// root (mounting the netrc secret, which a build without one still runs),
// the lists, the project's files, and an ignore file keeping the Dockerfile
// out of the image. The project's own root Dockerfile is not carried, here or
// in Build, so the two make the same image.
func TestWriteContext(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"dags/a.py":        "x = 1\n",
		"pyproject.toml":   "[project]\nname = 'p'\n",
		"Dockerfile":       "FROM somewhere\n",
		"packages.txt":     "stale\n",
		".dockerignore":    "secret.txt\n",
		"secret.txt":       "never\n",
		"include/notes.md": "carried\n",
	})
	req := testRequest(t)
	req.ProjectFiles = project
	req.Install = &InstallLists{Requirements: []string{"pandas==2.2.3"}, Packages: []string{"libpq-dev"}}

	dir := filepath.Join(t.TempDir(), "build")
	require.NoError(t, WriteContext(dir, req))
	assert.ElementsMatch(t, []string{
		"dags/a.py", "pyproject.toml", "include/notes.md",
		"Dockerfile", ".dockerignore", requirementsName, packagesName, lockConstraintsName, routedName, routedLinksName,
	}, contextFiles(t, dir))
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		return string(data)
	}
	assert.Equal(t, GeneratedDockerfile(req.BaseImage, true), read("Dockerfile"))
	assert.Contains(t, read("Dockerfile"), "--mount=type=secret,id=netrc,target=/root/.netrc,required=false")
	assert.Equal(t, "Dockerfile\n.dockerignore\n", read(".dockerignore"))
	assert.Equal(t, "libpq-dev\n", read(packagesName))
	assert.Equal(t, "pandas==2.2.3\n", read(requirementsName))

	// The same files Build lays down for the image, besides the two that
	// stay out of it.
	_, err := testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"dags/a.py", "pyproject.toml", "include/notes.md", requirementsName, packagesName, lockConstraintsName, routedName, routedLinksName,
	}, contextFiles(t, filepath.Join(req.WorkDir, buildContextDir)))
}

// A context is written only into a directory that is new or empty, and only
// for a generated build.
func TestWriteContextRefuses(t *testing.T) {
	req := testRequest(t)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"keep.txt": "mine\n"})
	err := WriteContext(dir, req)
	require.ErrorIs(t, err, ErrContextDirNotEmpty)
	assert.FileExists(t, filepath.Join(dir, "keep.txt"))

	empty := t.TempDir()
	require.NoError(t, WriteContext(empty, req), "an empty directory is fine")

	declared := Request{Dockerfile: filepath.Join(dir, "Dockerfile"), Context: dir}
	assert.Error(t, WriteContext(filepath.Join(t.TempDir(), "x"), declared))
}
