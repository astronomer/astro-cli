package imagebuild

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// hostPython is a Python that can run the install helper (3.11+, or one with
// tomli), or "" when the machine has none. The helper runs inside the build on
// the runtime image's Python; here it runs on the host's, so its output can be
// checked without a container engine. CI's runners all ship one.
func hostPython(t *testing.T) string {
	t.Helper()
	// The plain names first, then the versioned ones a machine whose python3
	// is too old to read TOML (macOS's own 3.9) has beside it.
	for _, name := range []string{"python3", "python", "python3.14", "python3.13", "python3.12", "python3.11"} {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		// The helper also needs packaging, its own or pip's vendored copy, as
		// every runtime image has.
		if exec.Command(bin, "-c", "import sys; assert sys.version_info >= (3, 8)\ntry:\n import tomllib\nexcept ImportError:\n import tomli\ntry:\n import packaging.markers\nexcept ImportError:\n import pip._vendor.packaging.markers").Run() == nil {
			return bin
		}
	}
	return ""
}

// installLists runs the install helper over the build context in dir, as the
// generated Dockerfile's install stage does, and returns what it wrote:
// requirements.txt, packages.txt and the lock's constraints. It skips the
// test when the host has no Python or uv to run it with.
func installLists(t *testing.T, dir string) (requirements, packages, constraints string) {
	t.Helper()
	out := runHelper(t, dir, helperRun{})
	return out.requirements, out.packages, out.constraints
}

// helperRun is how runHelper runs the helper: the runtime constraints file it
// reads ("" for none), and environment variables besides the test's own.
type helperRun struct {
	runtimeConstraints string
	env                []string
}

// helperOutput is what one run of the helper wrote, and its warnings.
type helperOutput struct {
	requirements, packages, constraints, stderr string
}

// runHelper runs the install helper over dir as the install stage does, with
// the host's uv as the stage's, skipping the test when the host has no Python
// or uv to run it with. uv runs offline unless r.env says otherwise: a lock
// that is current needs no network, and one that is not fails to resolve
// again, as in a build that cannot reach an index.
func runHelper(t *testing.T, dir string, r helperRun) helperOutput {
	t.Helper()
	py := hostPython(t)
	if py == "" {
		t.Skip("no Python 3.8+ with tomllib or tomli on PATH to run the install helper on")
	}
	uvBin, err := exec.LookPath("uv")
	if err != nil {
		t.Skip("no uv on PATH to run the install helper with")
	}
	if r.runtimeConstraints == "" {
		// An empty one, as a runtime that pins nothing would have. A missing
		// one is a runtime before 11.0.0.
		r.runtimeConstraints = filepath.Join(t.TempDir(), "pip-constraints.txt")
		require.NoError(t, os.WriteFile(r.runtimeConstraints, nil, 0o600))
	}
	out := t.TempDir()
	var stderr bytes.Buffer
	// A project's own build reads its inputs from the project; a generated
	// build's context has them in generatedInputsDir, as its stage copies them.
	project, helper := dir, filepath.Join(dir, filepath.FromSlash(InstallHelperPath))
	if inputs := filepath.Join(dir, filepath.FromSlash(generatedInputsDir)); dirExists(inputs) {
		project, helper = inputs, filepath.Join(inputs, filepath.Base(InstallHelperPath))
	}
	cmd := exec.Command(py, helper, "--project", project, "--out", out, "--runtime-constraints", r.runtimeConstraints)
	// The host's own packages are not a runtime image's: none, unless the test
	// names them. uv reads no settings of the host's, and resolves for the
	// Python the helper runs on.
	cmd.Env = append(append(os.Environ(), "ASTRO_INSTALL_PROJECT_BASE_INSTALLED=", "ASTRO_UV="+uvBin,
		"UV_OFFLINE=1", "UV_NO_CONFIG=1", "UV_CACHE_DIR="+t.TempDir(), "UV_PYTHON="+py, "UV_PYTHON_DOWNLOADS=never"), r.env...)
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), stderr.String())
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(out, name))
		require.NoError(t, err, name)
		return string(data)
	}
	return helperOutput{read(requirementsName), read(packagesName), read(lockConstraintsName), strings.ReplaceAll(stderr.String(), "\r\n", "\n")}
}

// helperProject is a build context holding the install helper and the given
// pyproject.toml and, when lock is not "", uv.lock.
func helperProject(t *testing.T, pyproject, lock string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{"pyproject.toml": pyproject, InstallHelperPath: string(installHelper)}
	if lock != "" {
		files["uv.lock"] = lock
	}
	writeTree(t, dir, files)
	return dir
}

// fixtureProject is helperProject over testdata/install/name: its
// pyproject.toml, the uv.lock `uv lock` wrote for it, and any file a path
// source names. Rewrite a lock with `uv lock` in its directory after changing
// its pyproject.toml.
func fixtureProject(t *testing.T, name string) string {
	t.Helper()
	pyproject, lock := fixtureFiles(t, name)
	dir := helperProject(t, pyproject, lock)
	root := filepath.Join("testdata", "install", name)
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Dir(path) == root {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		writeTree(t, dir, map[string]string{filepath.ToSlash(rel): string(data)})
		return nil
	}))
	return dir
}

// fixtureFiles are testdata/install/name's pyproject.toml and uv.lock.
func fixtureFiles(t *testing.T, name string) (pyproject, lock string) {
	t.Helper()
	read := func(file string) string {
		data, err := os.ReadFile(filepath.Join("testdata", "install", name, file))
		require.NoError(t, err)
		return string(data)
	}
	return read("pyproject.toml"), read("uv.lock")
}

// With a current uv.lock, what the runtime image ships stays as it ships it,
// and the rest is the lock's: the project's own packages (its dependencies,
// extras and dependency groups) by name at the lock's versions, and every
// other locked package the image lacks through the constraints file. The
// runtime's own pins come before either, through its constraints file and the
// files that names.
func TestInstallHelperInstallsTheLock(t *testing.T) {
	dir := fixtureProject(t, "pins")
	pins := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(pins, "pip-constraints.txt"), []byte("# the runtime's\n-c more.txt\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pins, "more.txt"), []byte("urllib3==2.5.0\n"), 0o600))

	out := runHelper(t, dir, helperRun{
		runtimeConstraints: filepath.Join(pins, "pip-constraints.txt"),
		env:                []string{"ASTRO_INSTALL_PROJECT_BASE_INSTALLED=requests,idna,certifi,urllib3"},
	})
	assert.Empty(t, out.stderr)
	assert.Equal(t, "-c uv-lock-constraints.txt\nidna==3.20\nrequests==2.32.3\ntabulate==0.10.0\n", out.requirements,
		"requests and idna (an extra's) are the project's own though the base has them")
	assert.Equal(t, "charset-normalizer==3.5.2\n", out.constraints, "certifi is the base's, urllib3 the runtime's")
	assert.Empty(t, out.packages)
}

// A package the lock takes from an index other than PyPI is installed from the
// file the lock recorded, by URL and hash, so the install never asks PyPI for
// it. PyPI's are installed by name.
func TestInstallHelperInstallsAnIndexSourceFromItsFile(t *testing.T) {
	out := runHelper(t, fixtureProject(t, "index"), helperRun{})
	assert.Empty(t, out.stderr)
	assert.Equal(t, "six @ https://test-files.pythonhosted.org/packages/1e/5c/ecacc0c9c346004e43acc024f076d74c0005d11b9a70ecaa81b21c8b080b/six-1.16.0-py2.py3-none-any.whl"+
		"#sha256=8a8a628020883f98172cef8ab824189aeaa1dde28720cc90e367bb1103ab7a58\ntabulate==0.10.0\n", out.requirements)
}

// A package from an index no [tool.uv.sources] entry routes it to (a default
// index of the project's own, or the runtime image's, which a lock resolved in
// the build reads) is installed by name, through the image's own index, and a
// package the base image has stays the base's.
func TestInstallHelperInstallsByNameFromADefaultIndex(t *testing.T) {
	out := runHelper(t, fixtureProject(t, "mirror"), helperRun{})
	assert.Empty(t, out.stderr)
	assert.Equal(t, "six==1.16.0\n", out.requirements)
}

// A package from a path is installed by name at the lock's version, as the
// path is not in the image.
func TestInstallHelperInstallsAPathSourceByName(t *testing.T) {
	out := runHelper(t, fixtureProject(t, "path"), helperRun{})
	assert.Empty(t, out.stderr)
	assert.Equal(t, "localpkg==1.2.0\ntabulate==0.10.0\n", out.requirements)
}

// A git or URL source is a direct reference at the commit or file the lock
// recorded, without the user and password a URL carries: what the helper
// writes lands in an image layer and the build log, and the build
// authenticates through the netrc secret. A git source brings git in through
// packages.txt, since the runtime image ships none. uv checks such a lock
// against its sources over the network.
func TestInstallHelperInstallsGitAndURLSources(t *testing.T) {
	if testing.Short() {
		t.Skip("reads a git repository and a file from the network")
	}
	out := runHelper(t, fixtureProject(t, "sources"), helperRun{env: []string{"UV_OFFLINE=false"}})
	assert.Equal(t, "pyfiglet @ git+https://github.com/pwaller/pyfiglet@be130d640d520d4a9db2b4f8a86f9d838534f8d0\n"+
		"six @ https://files.pythonhosted.org/packages/b7/ce/149a00dd41f10bc29e5921b496af8b574d8413afcd5e30dfa0ed46c2cc5e/six-1.17.0-py2.py3-none-any.whl\n", out.requirements)
	assert.NotContains(t, out.requirements, "s3cret")
	assert.Equal(t, 1, strings.Count(out.stderr, "user and password"), out.stderr)
	assert.Equal(t, "git\n", out.packages)
}

// A lock that no longer matches pyproject.toml, or no lock, is resolved again
// in the build, with a warning. When that cannot be done either (here, offline)
// the build installs pyproject.toml's dependencies as written, without Airflow
// or a Task SDK the project gives no version, and says so.
func TestInstallHelperInstallsUnpinnedWhenUvCannotLock(t *testing.T) {
	_, stale := fixtureFiles(t, "pins")
	const pyproject = "[project]\nname = 'p'\nversion = '0'\n" +
		"dependencies = ['apache-airflow==3.1.*', 'Apache_Airflow-Core', 'apache-airflow-task-sdk', 'pandas>=2', 'x @ git+https://example.com/x']\n" +
		"[tool.astro]\npackages = ['libpq-dev']\n"
	for name, lock := range map[string]string{"no lock": "", "a stale lock": stale} {
		t.Run(name, func(t *testing.T) {
			out := runHelper(t, helperProject(t, pyproject, lock), helperRun{})
			assert.Contains(t, out.stderr, "uv.lock is missing or out of date with pyproject.toml; resolving it in the build")
			assert.Contains(t, out.stderr, "uv could not lock the project")
			assert.Equal(t, "pandas>=2\nx @ git+https://example.com/x\n", out.requirements)
			assert.Empty(t, out.constraints)
			assert.Equal(t, "libpq-dev\ngit\n", out.packages)
		})
	}
	out := runHelper(t, helperProject(t, "[project]\nname = 'p'\nversion = '0'\ndependencies = ['apache-airflow-task-sdk>=1.1']\n", ""), helperRun{})
	assert.Equal(t, "apache-airflow-task-sdk>=1.1\n", out.requirements, "a Task SDK with a version stays")
}

// A project that keeps its files read-only ships them read-only into the
// context, and the build still writes its own install inputs over them.
func TestBuildReplacesReadOnlyProjectFiles(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"pyproject.toml":  "[project]\nname = 'p'\ndependencies = ['pandas']\n",
		"uv.lock":         "version = 1\n",
		InstallHelperPath: "# an older helper\n",
	})
	for _, name := range []string{"pyproject.toml", "uv.lock", InstallHelperPath} {
		require.NoError(t, os.Chmod(filepath.Join(project, filepath.FromSlash(name)), 0o444))
	}
	req := testRequest(t)
	req.ManifestDir, req.ProjectFiles = project, project
	req.Dependencies = []string{"pandas"}

	_, err := testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	helper, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, filepath.FromSlash(generatedInputsDir), filepath.Base(InstallHelperPath)))
	require.NoError(t, err)
	assert.Equal(t, InstallHelper(), helper, "the CLI's helper, not the project's")
}

// With a project's pyproject.toml to install from, that file decides whether
// there is anything to install, not the request's own lists.
func TestBuildFastPathFollowsTheProjectsManifest(t *testing.T) {
	for name, tc := range map[string]struct {
		pyproject    string
		dependencies []string
		builds       bool
	}{
		"the project lists a dependency the request does not": {"[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*', 'pandas']\n[tool.astro]\n", nil, true},
		"the project lists an OS package":                     {"[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*']\n[tool.astro]\npackages = ['libpq-dev']\n", nil, true},
		"the project lists only Airflow":                      {"[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*']\n[tool.astro]\n", []string{"pandas"}, false},
		"a pyproject.toml that does not parse":                {"[project\n", nil, true},
	} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			writeTree(t, project, map[string]string{"pyproject.toml": tc.pyproject})
			cmd := &fakeCmd{}
			req := testRequest(t)
			req.ManifestDir, req.Dependencies = project, tc.dependencies
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

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// An older build left a find-links file in a project's .astro. It is taken
// out of the context, and never read.
func TestBuildDropsAFindLinksFileTheProjectCarries(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"pyproject.toml":                 "[project]\nname = 'p'\ndependencies = ['pandas']\n[tool.astro]\n",
		".astro/" + installFindLinksName: "https://evil.example/simple/pandas/\n",
	})
	req := testRequest(t)
	req.ManifestDir, req.ProjectFiles = project, project
	req.Dependencies = []string{"pandas"}

	_, err := testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	ctxDir := filepath.Join(req.WorkDir, buildContextDir)
	assert.NoFileExists(t, filepath.Join(ctxDir, ".astro", installFindLinksName))
	assert.NoFileExists(t, filepath.Join(ctxDir, filepath.FromSlash(generatedInputsDir), installFindLinksName))
	reqs, _, _ := installLists(t, ctxDir)
	assert.Equal(t, "pandas\n", reqs)
}
