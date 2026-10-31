package imageinstall

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// testResolver resolves with the host's uv, offline and reading no settings of
// the host's, against a runtime whose constraints file holds pins and whose
// image has installed (nil: an image that publishes no list). It skips the
// test on a machine without uv.
func testResolver(t *testing.T, pins, installed []string) Resolver {
	t.Helper()
	t.Setenv("UV_OFFLINE", "1")
	t.Setenv("UV_NO_CONFIG", "1")
	t.Setenv("UV_PYTHON_DOWNLOADS", "never")
	cache := t.TempDir()
	client, err := uv.New(t.Context(), uv.Options{CacheDir: cache})
	if errors.Is(err, uv.ErrNotFound) {
		t.Skip("no uv on PATH to resolve the project with")
	}
	require.NoError(t, err)
	return Resolver{
		UV: func(context.Context) (UV, error) { return client, nil },
		Constraints: func(context.Context, string, string) ([]string, error) {
			return pins, nil
		},
		Freeze: func(context.Context, string, string) ([]string, error) {
			if installed == nil {
				return nil, errors.New("not published")
			}
			return installed, nil
		},
	}
}

// fixture is a copy of testdata/install/name, which a resolve may write a
// uv.lock into. Rewrite a fixture's lock with `uv lock` in its directory after
// changing its pyproject.toml.
func fixture(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join("testdata", "install", name)
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		dst := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o600)
	}))
	return dir
}

// project is a project of the given pyproject.toml and, when lock is not "",
// uv.lock.
func project(t *testing.T, pyproject, lock string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyproject), 0o600))
	if lock != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "uv.lock"), []byte(lock), 0o600))
	}
	return dir
}

// fill runs r over a generated build of dir on runtime 3.1-5, with packages,
// and returns the request and the warnings.
func fill(t *testing.T, r Resolver, dir string, packages ...string) (req imagebuild.Request, warnings []string) {
	t.Helper()
	req = imagebuild.Request{BaseImage: imagebuild.RuntimeImageRepo + ":3.1-5-python-3.12", ManifestDir: dir, Packages: packages}
	require.NoError(t, r.Fill(t.Context(), &req, func(w string) { warnings = append(warnings, w) }))
	return req, warnings
}

// With a current uv.lock, what the runtime image ships stays as it ships it,
// and the rest is the lock's: the project's own packages (its dependencies,
// extras and dependency groups) by name at the lock's versions, and every
// other locked package the image lacks through the constraints file. The
// runtime's own pins come before either.
func TestFillInstallsTheLock(t *testing.T) {
	r := testResolver(t, []string{"urllib3==2.5.0"}, []string{"requests==2.31.0", "idna==3.7", "certifi==2024.1.1", "urllib3==2.5.0"})
	req, warnings := fill(t, r, fixture(t, "pins"), "libpq-dev")
	assert.Empty(t, warnings)
	require.NotNil(t, req.Install)
	assert.Equal(t, []string{"idna==3.20", "requests==2.32.3", "tabulate==0.10.0"}, req.Install.Requirements,
		"requests and idna (an extra's) are the project's own though the base has them")
	assert.Equal(t, []string{"charset-normalizer==3.5.2"}, req.Install.Constraints, "certifi is the base's, urllib3 the runtime's")
	assert.Equal(t, []string{"libpq-dev"}, req.Install.Packages)
}

// When the image's package list cannot be read, the project's own packages
// are still the lock's, and nothing else is constrained: the install keeps
// what the image has.
func TestFillWithoutTheImagesPackages(t *testing.T) {
	r := testResolver(t, nil, nil)
	req, warnings := fill(t, r, fixture(t, "pins"))
	require.NotNil(t, req.Install)
	assert.Equal(t, []string{"idna==3.20", "requests==2.32.3", "tabulate==0.10.0"}, req.Install.Requirements)
	assert.Empty(t, req.Install.Constraints)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "the package list of runtime 3.1-5-python-3.12 could not be read")
}

// A package the lock takes from an index other than PyPI installs on its own,
// from the files the lock recorded, by URL and hash, so the install never asks
// the image's index for it. PyPI's are installed by name.
func TestFillInstallsAnIndexSourceFromItsFiles(t *testing.T) {
	req, warnings := fill(t, testResolver(t, nil, []string{}), fixture(t, "index"))
	assert.Empty(t, warnings)
	assert.ElementsMatch(t, []string{"six==1.16.0", "tabulate==0.10.0"}, req.Install.Requirements)
	assert.Equal(t, []string{"six==1.16.0"}, req.Install.Routed)
	assert.Contains(t, req.Install.RoutedLinks,
		"https://test-files.pythonhosted.org/packages/1e/5c/ecacc0c9c346004e43acc024f076d74c0005d11b9a70ecaa81b21c8b080b/six-1.16.0-py2.py3-none-any.whl"+
			"#sha256=8a8a628020883f98172cef8ab824189aeaa1dde28720cc90e367bb1103ab7a58")
	for _, l := range req.Install.RoutedLinks {
		assert.Contains(t, l, "https://test-files.pythonhosted.org/", "only the routed index's files")
	}
}

// A package from an index no [tool.uv.sources] entry routes it to (a default
// index of the project's own) is installed by name, through the image's own
// index.
func TestFillInstallsByNameFromADefaultIndex(t *testing.T) {
	req, warnings := fill(t, testResolver(t, nil, []string{}), fixture(t, "mirror"))
	assert.Empty(t, warnings)
	assert.Equal(t, []string{"six==1.16.0"}, req.Install.Requirements)
}

// A package from a path on this machine is refused before the build, by
// name and path: the image cannot reach it, and installing the name from the
// image's index could install someone else's package of that name.
func TestFillRefusesAPathSource(t *testing.T) {
	req := imagebuild.Request{BaseImage: imagebuild.RuntimeImageRepo + ":3.1-5-python-3.12", ManifestDir: fixture(t, "path")}
	err := testResolver(t, nil, []string{}).Fill(t.Context(), &req, nil)
	require.ErrorIs(t, err, ErrLocalSource)
	assert.Contains(t, err.Error(), "localpkg (stub/localpkg-1.2.0-py3-none-any.whl)")
	assert.Nil(t, req.Install)
}

// A git or URL source is a direct reference at the commit or file the lock
// recorded, without the user and password a URL carries: the lists land in an
// image layer and the build log, and the build authenticates through the
// netrc secret. A git source brings git in through packages.txt, since the
// runtime image ships none. uv checks such a lock against its sources over the
// network.
func TestFillInstallsGitAndURLSources(t *testing.T) {
	if testing.Short() {
		t.Skip("reads a git repository and a file from the network")
	}
	r := testResolver(t, nil, []string{})
	t.Setenv("UV_OFFLINE", "false")
	req, warnings := fill(t, r, fixture(t, "sources"))
	require.NotNil(t, req.Install, "%v", warnings)
	assert.Equal(t, []string{
		"pyfiglet @ git+https://github.com/pwaller/pyfiglet@be130d640d520d4a9db2b4f8a86f9d838534f8d0",
		"six @ https://files.pythonhosted.org/packages/b7/ce/149a00dd41f10bc29e5921b496af8b574d8413afcd5e30dfa0ed46c2cc5e/six-1.17.0-py2.py3-none-any.whl",
	}, req.Install.Requirements)
	assert.NotContains(t, strings.Join(req.Install.Requirements, "\n"), "s3cret")
	assert.Equal(t, []string{"the image build leaves out a URL's user and password: give the build a netrc secret instead"}, warnings)
	assert.Equal(t, []string{"git"}, req.Install.Packages)
}

// No lock is written with uv lock, with a warning. When that cannot be done
// either (here, offline) the build installs the request's dependencies as
// written, and says so.
func TestFillLeavesTheRequestUnpinnedWhenUvCannotLock(t *testing.T) {
	const pyproject = "[project]\nname = 'p'\nversion = '0'\ndependencies = ['pandas>=2']\n"
	req, warnings := fill(t, testResolver(t, nil, []string{}), project(t, pyproject, ""))
	assert.Nil(t, req.Install)
	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "uv.lock is missing or out of date with pyproject.toml, so the build is running uv lock in the project")
	assert.Contains(t, warnings[1], "uv could not lock the project")
}

// A committed lock uv can neither check nor relock (here, offline) and that
// does not even export (it was written for another project) leaves the
// request unpinned, with each reason said; it is not an error.
func TestFillLeavesTheRequestUnpinnedWhenTheLockCannotBeRead(t *testing.T) {
	other, err := os.ReadFile(filepath.Join("testdata", "install", "pins", "uv.lock"))
	require.NoError(t, err)
	const pyproject = "[project]\nname = 'p'\nversion = '0'\ndependencies = ['pandas>=2']\n"
	req, warnings := fill(t, testResolver(t, nil, []string{}), project(t, pyproject, string(other)))
	assert.Nil(t, req.Install)
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[len(warnings)-1], "uv could not read the committed uv.lock either")
}

// A stale lock is written again in the project, and the image installs what
// the new one says.
func TestFillRelocksAStaleLock(t *testing.T) {
	fake := &fakeUV{check: fmt.Errorf("%w: uv lock --check failed", uv.ErrLockStale), pylock: "[[packages]]\nname = 'pandas'\nversion = '2.2.3'\n"}
	r := Resolver{UV: func(context.Context) (UV, error) { return fake, nil }}
	dir := project(t, "[project]\nname = 'p'\ndependencies = ['pandas']\n", "")

	req, warnings := fill(t, r, dir)
	assert.Equal(t, []string{"check " + dir, "lock " + dir, "export " + dir}, fake.calls)
	require.Len(t, warnings, 2, "%v", warnings)
	assert.Contains(t, warnings[0], "commit the uv.lock it writes")
	assert.Contains(t, warnings[1], "could not be read", "no runtime files here")
	require.NotNil(t, req.Install)
	assert.Equal(t, []string{"pandas==2.2.3"}, req.Install.Requirements)
}

// A check uv could not make is not a stale lock: the committed lock is
// installed as it stands, and uv lock is not run in the project.
func TestFillDoesNotRelockWhenTheCheckFails(t *testing.T) {
	fake := &fakeUV{check: errors.New("uv lock failed (exit 2): Request failed after 3 retries"), pylock: "[[packages]]\nname = 'pandas'\nversion = '2.2.3'\n"}
	r := Resolver{UV: func(context.Context) (UV, error) { return fake, nil }}
	dir := project(t, "[project]\nname = 'p'\ndependencies = ['pandas']\n", "version = 1\n")

	req, warnings := fill(t, r, dir)
	assert.Equal(t, []string{"check " + dir, "export " + dir}, fake.calls)
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "uv could not check uv.lock against pyproject.toml, so the image installs uv.lock as committed")
	require.NotNil(t, req.Install)
	assert.Equal(t, []string{"pandas==2.2.3"}, req.Install.Requirements)
}

// A stale lock uv cannot write again is installed as committed, with a
// warning, rather than dropped for an unpinned install.
func TestFillInstallsAStaleLockUvCannotRelock(t *testing.T) {
	fake := &fakeUV{check: uv.ErrLockStale, lock: errors.New("no solution"), pylock: "[[packages]]\nname = 'pandas'\nversion = '2.2.3'\n"}
	r := Resolver{UV: func(context.Context) (UV, error) { return fake, nil }}
	dir := project(t, "[project]\nname = 'p'\ndependencies = ['pandas']\n", "version = 1\n")

	req, warnings := fill(t, r, dir)
	assert.Equal(t, []string{"check " + dir, "lock " + dir, "export " + dir}, fake.calls)
	require.GreaterOrEqual(t, len(warnings), 2)
	assert.Contains(t, warnings[1], "installs uv.lock as committed, without what pyproject.toml changed since")
	require.NotNil(t, req.Install)
}

// fakeUV answers the check with check and the lock with lock, and exports
// pylock, recording each call and the project it was for.
type fakeUV struct {
	check  error
	lock   error
	pylock string
	calls  []string
}

func (f *fakeUV) LockCheck(_ context.Context, project string) error {
	f.calls = append(f.calls, "check "+project)
	return f.check
}

func (f *fakeUV) Lock(_ context.Context, project string, _ uv.Stdio) error {
	f.calls = append(f.calls, "lock "+project)
	return f.lock
}

func (f *fakeUV) ExportPylock(_ context.Context, project string) ([]byte, error) {
	f.calls = append(f.calls, "export "+project)
	return []byte(f.pylock), nil
}

// Without uv the request installs as written, and the build says why.
func TestFillWithoutUV(t *testing.T) {
	r := Resolver{UV: func(context.Context) (UV, error) { return nil, uv.ErrNotFound }}
	req, warnings := fill(t, r, project(t, "[project]\nname = 'p'\n", ""))
	assert.Nil(t, req.Install)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "uv is needed to read the lock, and it was not found")
}

// A project's own Dockerfile built on the install base is resolved as a
// generated build is, against the runtime the base is built on, the build and
// Python both read from it.
func TestFillResolvesADeclaredDockerfileAgainstItsInstallBase(t *testing.T) {
	r := testResolver(t, []string{"urllib3==2.5.0"}, []string{"requests==2.31.0", "idna==3.7", "certifi==2024.1.1", "urllib3==2.5.0"})
	var asked []string
	freeze := r.Freeze
	r.Freeze = func(ctx context.Context, build, python string) ([]string, error) {
		asked = append(asked, build+" "+python)
		return freeze(ctx, build, python)
	}
	dir := fixture(t, "pins")
	df := filepath.Join(dir, "Dockerfile")
	require.NoError(t, os.WriteFile(df, []byte("ARG ASTRO_BASE\nFROM ${ASTRO_BASE}\nRUN true\n"), 0o600))
	req := imagebuild.Request{Dockerfile: df, Context: dir, ManifestDir: dir, BaseImage: imagebuild.RuntimeImageRepo + ":3.1-5-python-3.12"}
	var warnings []string
	require.NoError(t, r.Fill(t.Context(), &req, func(w string) { warnings = append(warnings, w) }))
	assert.Empty(t, warnings)
	require.NotNil(t, req.Install)
	assert.Equal(t, []string{"idna==3.20", "requests==2.32.3", "tabulate==0.10.0"}, req.Install.Requirements)
	assert.Equal(t, []string{"3.1-5 3.12"}, asked)
}

// Nothing to resolve: a declared Dockerfile that is not built on the install
// base, a request naming no project, a project with no pyproject.toml. uv
// is not asked.
func TestFillLeavesWhatItCannotResolve(t *testing.T) {
	r := Resolver{UV: func(context.Context) (UV, error) {
		t.Fatal("uv asked for")
		return nil, nil
	}}
	for name, req := range map[string]imagebuild.Request{
		"declared Dockerfile": {Dockerfile: "/p/Dockerfile", Context: "/p", ManifestDir: t.TempDir()},
		"no project":          {BaseImage: "runtime:3.1"},
		"no pyproject.toml":   {BaseImage: "runtime:3.1", ManifestDir: t.TempDir()},
	} {
		require.NoError(t, r.Fill(t.Context(), &req, nil), name)
		assert.Nil(t, req.Install, name)
	}
}

// The runtime build and Python an image is of, from its tag and the catalog.
func TestRuntimeOf(t *testing.T) {
	r := Resolver{}
	for image, want := range map[string][2]string{
		"astrocrpublic.azurecr.io/runtime:3.1-5-python-3.12":      {"3.1-5", "3.12"},
		"astrocrpublic.azurecr.io/runtime:3.1-5-python-3.12-base": {"3.1-5", "3.12"},
		"quay.io/astronomer/astro-runtime:12.6.0":                 {"12.6.0", "3.12"},
		"quay.io/astronomer/astro-runtime:10.9.0-base":            {"10.9.0", "3.11"},
		"astrocrpublic.azurecr.io/runtime:3.1":                    {"", ""},
		"registry.example:5000/runtime":                           {"", ""},
	} {
		build, python := r.runtimeOf(t.Context(), image)
		assert.Equal(t, want, [2]string{build, python}, image)
	}
}

// A lock made for another Airflow release than the image's base runs refuses
// the build before it starts, naming both Airflows, the runtime
// pyproject.toml records for the lock, and astro init. Another Astronomer
// build of the same release, or PyPI's, is no other Airflow; an image whose
// Airflow is not known (no package list, no catalog) is not held to it. The
// fixture's Airflow is a stub wheel, so its lock is small.
func TestFillRefusesALockForAnotherAirflow(t *testing.T) {
	for _, tt := range []struct {
		name      string
		installed []string
		want      []string
	}{
		{name: "same airflow", installed: []string{"apache-airflow==3.3.2+astro.2"}},
		{name: "same release, another astro build", installed: []string{"apache-airflow==3.3.2+astro.3"}},
		{name: "same release from pypi", installed: []string{"Apache_Airflow==3.3.2"}},
		{name: "image airflow unknown"},
		{
			name: "another airflow", installed: []string{"six==1.17.0", "apache-airflow==3.3.3+astro.1"},
			want: []string{
				"uv.lock was made for Airflow 3.3.2+astro.2 (Astro Runtime 3.3-9), and the image builds on runtime 3.1-5-python-3.12, which has Airflow 3.3.3+astro.1",
				"To keep the lock's Airflow, set [tool.astro] runtime in pyproject.toml to a build with Airflow 3.3.2, such as 3.3-9 (astro local upgrade runtime <build>); to move to the image's, run astro init",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := testResolver(t, nil, tt.installed)
			req := imagebuild.Request{BaseImage: imagebuild.RuntimeImageRepo + ":3.1-5-python-3.12", ManifestDir: fixture(t, "airflow")}
			err := r.Fill(t.Context(), &req, nil)
			if len(tt.want) == 0 {
				require.NoError(t, err)
				require.NotNil(t, req.Install)
				return
			}
			var mismatch *AirflowMismatchError
			require.ErrorAs(t, err, &mismatch)
			for _, s := range tt.want {
				assert.Contains(t, err.Error(), s)
			}
			assert.Nil(t, req.Install, "nothing is handed to a build that is refused")
		})
	}
}

// The image's Airflow comes from the catalog when its package list cannot be
// read, and the lock's runtime is named only from the record astro writes.
func TestFillReadsTheImagesAirflowFromTheCatalog(t *testing.T) {
	r := testResolver(t, nil, nil)
	catalog, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {"3.1-5": {"metadata": ` +
		`{"airflowVersion": "3.1.5", "channel": "stable", "releaseDate": "2025-10-01", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}}}}`))
	require.NoError(t, err)
	r.Catalog = func(context.Context) *runtimeversions.Catalog { return catalog }
	dir := fixture(t, "airflow")
	pyproject := filepath.Join(dir, "pyproject.toml")
	data, err := os.ReadFile(pyproject)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pyproject, []byte(strings.Replace(string(data),
		"    # Astro Runtime 3.3-9 (Python 3.12) constrains these. astro writes them.\n", "", 1)), 0o600))
	req := imagebuild.Request{BaseImage: imagebuild.RuntimeImageRepo + ":3.1-5-base", ManifestDir: dir}
	err = r.Fill(t.Context(), &req, nil)
	var mismatch *AirflowMismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Contains(t, err.Error(), "uv.lock was made for Airflow 3.3.2+astro.2, and the image builds on runtime 3.1-5, which has Airflow 3.1.5")
}

// A mismatch names both ways out: [tool.astro] runtime, which picks the image's
// runtime for a generated build and a declared Dockerfile's install base alike,
// to keep the lock's Airflow, and astro init to take the image's.
func TestAirflowMismatchNamesBothWaysOut(t *testing.T) {
	err := &AirflowMismatchError{Locked: "3.3.2+astro.2", Image: "3.1.7+astro.1", Recorded: "3.3-11", Base: "quay.io/astronomer/runtime:3.1-12-base"}
	assert.Contains(t, err.Error(), "uv.lock was made for Airflow 3.3.2+astro.2 (Astro Runtime 3.3-11), and the image builds on runtime 3.1-12, which has Airflow 3.1.7+astro.1")
	assert.Contains(t, err.Error(), "To keep the lock's Airflow, set [tool.astro] runtime in pyproject.toml to a build with Airflow 3.3.2, such as 3.3-11 "+
		"(astro local upgrade runtime <build>); to move to the image's, run astro init, which relocks uv.lock")
}
