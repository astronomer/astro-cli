package runtimecatalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rootsServer is freezeServer with the published lists and PyPI's index
// pages served beside it.
func rootsServer(t *testing.T, files map[string]string) {
	t.Helper()
	srv, _ := freezeServer(t, files)
	oldRoots, oldPyPI := rootsBaseURL, pypiSimpleURL
	rootsBaseURL, pypiSimpleURL = srv.URL+"/runtime-roots", srv.URL+"/simple"
	t.Cleanup(func() { rootsBaseURL, pypiSimpleURL = oldRoots, oldPyPI })
}

// indexPage is PyPI's JSON index page listing files.
func indexPage(files ...string) string {
	quoted := make([]string, len(files))
	for i, f := range files {
		quoted[i] = `{"filename": "` + f + `"}`
	}
	return `{"files": [` + strings.Join(quoted, ",") + `]}`
}

const freeze339py314 = "apache-airflow==3.3.2+astro.2\nstatsd==4.0.1\npsycopg2-binary==2.9.12\nmysqlclient==2.2.8\n" +
	"apache-airflow-providers-celery==3.24.1\nlinuxonly==1.0\nkubernetes==33.0.0\n"

// pypi339 are the index pages of freeze339py314's packages.
var pypi339 = map[string]string{
	"/simple/statsd/":                          indexPage("statsd-4.0.1-py2.py3-none-any.whl", "statsd-4.0.1.tar.gz"),
	"/simple/psycopg2-binary/":                 indexPage("psycopg2_binary-2.9.12-cp314-cp314-macosx_11_0_arm64.whl", "psycopg2_binary-2.9.12-cp314-cp314-manylinux_2_17_x86_64.whl"),
	"/simple/mysqlclient/":                     indexPage("mysqlclient-2.2.8-cp314-cp314-win_amd64.whl", "mysqlclient-2.2.8.tar.gz"),
	"/simple/apache-airflow-providers-celery/": indexPage("apache_airflow_providers_celery-3.24.1-py3-none-any.whl"),
	"/simple/linuxonly/":                       indexPage("linuxonly-1.0-cp314-cp314-manylinux_2_28_aarch64.whl"),
	"/simple/kubernetes/":                      indexPage("kubernetes-33.0.0-py2.py3-none-any.whl"),
}

// served is pypi339 and the freeze of 3.3-9 on Python 3.14, with more beside.
func served(more map[string]string) map[string]string {
	out := map[string]string{"/runtime-freeze/runtime-3.3-9-python-3.14.txt": freeze339py314}
	for _, m := range []map[string]string{pypi339, more} {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// A published list is read as names, less Airflow, pip and Astronomer's own,
// sorted, and kept; each is limited to the platforms its version has a wheel
// for on the image's Python, and one with none (mysqlclient, which only builds
// from source) is left out.
func TestRuntimeRootsReadsThePublishedList(t *testing.T) {
	rootsServer(t, served(map[string]string{
		"/runtime-roots/runtime-3.3-9-python-3.14.txt": "statsd==4.0.1\nPsycopg2_Binary==2.9.12\npip==26.2.1\nmysqlclient==2.2.8\nlinuxonly==1.0\n" +
			"astronomer-runtime-extensions==1.0.0\napache-airflow==3.3.2+astro.2\napache-airflow-providers-celery==3.24.1\n",
	}))
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14")
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow-providers-celery", "linuxonly; sys_platform == 'linux'", "psycopg2-binary", "statsd"}, got)
	_, err = os.Stat(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", rootsCacheDir, "runtime-3.3-9-python-3.14.txt"))
	assert.NoError(t, err, "the published list is kept")
}

// Without a published list, one already worked out is used, with no request
// to resolve again.
func TestRuntimeRootsUsesAComputedList(t *testing.T) {
	rootsServer(t, served(nil))
	dir := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", rootsComputedDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "runtime-3.3-9-python-3.14.txt"), []byte("kubernetes\nstatsd\n"), 0o600))
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14")
	require.NoError(t, err)
	assert.Equal(t, []string{"kubernetes", "statsd"}, got)
}

// A package whose index page cannot be read leaves the whole list unknown,
// which keeps the group as it is, rather than one without that package.
func TestRuntimeRootsNeedsEveryIndexPage(t *testing.T) {
	files := served(map[string]string{"/runtime-roots/runtime-3.3-9-python-3.14.txt": "statsd==4.0.1\nkubernetes==33.0.0\n"})
	delete(files, "/simple/kubernetes/")
	rootsServer(t, files)
	_, err := RuntimeRoots(context.Background(), "3.3-9", "3.14")
	assert.ErrorContains(t, err, "reading which platforms")
}

// A build with no freeze has no list to work out, which takes the group out.
func TestRuntimeRootsOfABuildWithNoFreeze(t *testing.T) {
	rootsServer(t, map[string]string{})
	_, err := RuntimeRoots(context.Background(), "10.9.0", "3.11")
	assert.ErrorIs(t, err, ErrFreezeNotPublished)
}

// Under a UV_EXCLUDE_NEWER cutoff the list is not read, as the freeze is not.
func TestRuntimeRootsUnderACutoff(t *testing.T) {
	rootsServer(t, map[string]string{})
	t.Setenv("UV_EXCLUDE_NEWER", "2026-01-01")
	_, err := RuntimeRoots(context.Background(), "3.3-9", "3.14")
	assert.ErrorIs(t, err, ErrPinnedResolution)
}

// The scratch project depends on every candidate and on the Airflow
// distributions the freeze has, whose sources need them direct; another of
// Astronomer's builds is no candidate.
func TestRootsProjectDeps(t *testing.T) {
	candidates, deps := rootsProjectDeps([]string{
		"apache-airflow==3.3.2+astro.2", "apache-airflow-task-sdk==1.3.2+astro.1", "pandas==2.3.3",
		"astronomer-kubernetes-executor==10.22.0+astro.1", "astronomer-fab-security-manager==1.10.0", "pip==26.2.1",
		"something==1.0+astro.1", "Statsd==4.0.1", "astronomer-providers==1.19.4",
	})
	// astronomer-providers is what a DAG imports; the rest plug into Airflow.
	assert.Equal(t, []string{"astronomer-providers", "pandas", "statsd"}, candidates)
	assert.Equal(t, []string{"astronomer-providers", "pandas", "statsd", "apache-airflow", "apache-airflow-task-sdk"}, deps)
}

// rootsFromLock counts a dependency and every package an extra it names
// installs, follows an extra that names its own package's extras once, and
// leaves out the scratch project's own edges.
func TestRootsFromLock(t *testing.T) {
	lock := `version = 1

[[package]]
name = "astro-runtime-roots"
version = "0"
source = { virtual = "." }
dependencies = [
    { name = "apache-airflow-core" },
    { name = "celery" },
    { name = "httptools" },
    { name = "kubernetes" },
    { name = "redis" },
    { name = "uvicorn" },
]

[[package]]
name = "apache-airflow-core"
version = "3.3.2+astro.2"
dependencies = [
    { name = "uvicorn", extra = ["standard"] },
]

[[package]]
name = "uvicorn"
version = "0.49.0"

[package.optional-dependencies]
standard = [
    { name = "httptools" },
    { name = "uvicorn", extra = ["all"] },
]
all = [
    { name = "uvicorn", extra = ["standard"] },
]

[[package]]
name = "celery"
version = "5.5.0"
dependencies = [
    { name = "kombu" },
]

[package.optional-dependencies]
redis = [
    { name = "redis" },
]

[[package]]
name = "httptools"
version = "0.8.0"

[[package]]
name = "kubernetes"
version = "33.0.0"

[[package]]
name = "redis"
version = "6.4.0"
`
	got, err := rootsFromLock([]byte(lock), []string{"celery", "httptools", "kubernetes", "redis", "uvicorn"})
	require.NoError(t, err)
	// celery's redis extra is named by nothing here, so redis is a root.
	assert.Equal(t, []string{"celery", "kubernetes", "redis"}, got)
}

// A wheel installs where its platform tag says, on a Python its tags fit: py3,
// this CPython, or an older CPython's stable ABI; not another CPython, nor
// this one's free-threaded build, nor a 32-bit platform.
func TestWheelPlatforms(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  int
	}{
		{"pure Python", []string{"x-1.0-py3-none-any.whl"}, onLinux | onDarwin},
		{"sdist only", []string{"x-1.0.tar.gz"}, 0},
		{"Windows only", []string{"x-1.0-cp314-cp314-win_amd64.whl"}, 0},
		{"another version", []string{"x-0.9-py3-none-any.whl"}, 0},
		{"another CPython", []string{"x-1.0-cp313-cp313-manylinux_2_17_x86_64.whl"}, 0},
		{"free-threaded", []string{"x-1.0-cp314-cp314t-macosx_11_0_arm64.whl"}, 0},
		{"stable ABI", []string{"x-1.0-cp39-abi3-manylinux_2_17_aarch64.manylinux2014_aarch64.whl"}, onLinux},
		{"Apple silicon", []string{"x-1.0-cp314-cp314-macosx_11_0_arm64.whl", "x-1.0-cp314-cp314-macosx_10_15_x86_64.whl"}, onDarwin},
		{"universal2 and musl", []string{"x-1.0-cp314-cp314-macosx_10_9_universal2.whl", "x-1.0-cp314-cp314-musllinux_1_2_x86_64.whl"}, onLinux | onDarwin},
		{"32-bit Linux", []string{"x-1.0-cp314-cp314-manylinux_2_17_i686.whl"}, 0},
		{"build tag", []string{"x-1.0-1-cp314-cp314-manylinux_2_17_x86_64.whl"}, onLinux},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, wheelPlatforms(tc.files, "1.0", "3.14"))
		})
	}
}
