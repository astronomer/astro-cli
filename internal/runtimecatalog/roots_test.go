package runtimecatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rootsServer is freezeServer with the published lists and the simple index's
// pages served beside it, and no index named in the environment. It returns
// the request count.
func rootsServer(t *testing.T, files map[string]string) *atomic.Int32 {
	t.Helper()
	srv, hits := freezeServer(t, files)
	for _, key := range []string{"UV_DEFAULT_INDEX", "UV_INDEX_URL", "PIP_INDEX_URL"} {
		t.Setenv(key, "")
	}
	oldRoots, oldPyPI := rootsBaseURL, pypiSimpleURL
	rootsBaseURL, pypiSimpleURL = srv.URL+"/runtime-roots", srv.URL+"/simple"
	t.Cleanup(func() { rootsBaseURL, pypiSimpleURL = oldRoots, oldPyPI })
	return hits
}

// indexPage is a simple index's JSON page listing files.
func indexPage(files ...string) string {
	quoted := make([]string, len(files))
	for i, f := range files {
		quoted[i] = `{"filename": "` + f + `"}`
	}
	return `{"files": [` + strings.Join(quoted, ",") + `]}`
}

var freeze339py314 = []string{
	"apache-airflow==3.3.2+astro.2", "statsd==4.0.1", "psycopg2-binary==2.9.12", "mysqlclient==2.2.8",
	"apache-airflow-providers-celery==3.24.1", "linuxonly==1.0", "kubernetes==33.0.0", "sdistonly==2.0",
}

// pypi339 are the index pages of freeze339py314's packages.
var pypi339 = map[string]string{
	"/simple/statsd/": indexPage("statsd-4.0.1-py2.py3-none-any.whl", "statsd-4.0.1.tar.gz"),
	"/simple/psycopg2-binary/": indexPage("psycopg2_binary-2.9.12-cp314-cp314-macosx_11_0_arm64.whl",
		"psycopg2_binary-2.9.12-cp314-cp314-manylinux_2_17_x86_64.whl", "psycopg2_binary-2.9.12-cp314-cp314-manylinux_2_28_aarch64.whl"),
	"/simple/mysqlclient/":                     indexPage("mysqlclient-2.2.8-cp314-cp314-win_amd64.whl", "mysqlclient-2.2.8.tar.gz"),
	"/simple/apache-airflow-providers-celery/": indexPage("apache_airflow_providers_celery-3.24.1-py3-none-any.whl"),
	"/simple/linuxonly/": indexPage("linuxonly-1.0-cp314-cp314-manylinux_2_28_aarch64.whl",
		"linuxonly-1.0-cp314-cp314-manylinux_2_17_x86_64.whl", "linuxonly-1.0-cp314-cp314-musllinux_1_2_x86_64.whl"),
	"/simple/kubernetes/": indexPage("kubernetes-33.0.0-py2.py3-none-any.whl"),
	"/simple/sdistonly/":  indexPage("sdistonly-2.0.tar.gz"),
}

// served is pypi339 with more beside it.
func served(more map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range []map[string]string{pypi339, more} {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

const linuxMarker = "(sys_platform == 'linux' and platform_machine == 'x86_64') or (sys_platform == 'linux' and platform_machine == 'aarch64')"

// A published list is read as names, less Airflow, pip and Astronomer's own,
// sorted, and kept. Each is limited to the platforms its version has a wheel
// for on the image's Python; a release with no wheel at all is pure Python;
// one with wheels for none of the platforms (mysqlclient's are Windows') is
// left out, and said so.
func TestRuntimeRootsReadsThePublishedList(t *testing.T) {
	rootsServer(t, served(map[string]string{
		"/runtime-roots/runtime-3.3-9-python-3.14.txt": "statsd==4.0.1\nPsycopg2_Binary==2.9.12\npip==26.2.1\nmysqlclient==2.2.8\nlinuxonly==1.0\nsdistonly==2.0\n" +
			"astronomer-runtime-extensions==1.0.0\napache-airflow==3.3.2+astro.2\napache-airflow-providers-celery==3.24.1\n",
	}))
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339py314)
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow-providers-celery", "linuxonly; " + linuxMarker, "psycopg2-binary", "sdistonly", "statsd"}, got.Names)
	assert.Equal(t, []string{"mysqlclient"}, got.LeftOut)
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
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339py314)
	require.NoError(t, err)
	assert.Equal(t, []string{"kubernetes", "statsd"}, got.Names)
}

// A package whose index page cannot be read leaves the whole list unknown,
// which keeps the group as it is, rather than one short of a package; and the
// failure is remembered for an hour, so the next start does not try again.
func TestRuntimeRootsRemembersAFailure(t *testing.T) {
	files := served(map[string]string{"/runtime-roots/runtime-3.3-9-python-3.14.txt": "statsd==4.0.1\nkubernetes==33.0.0\n"})
	delete(files, "/simple/kubernetes/")
	hits := rootsServer(t, files)
	_, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339py314)
	require.ErrorContains(t, err, "reading which platforms kubernetes has wheels for")
	before := hits.Load()
	_, err = RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339py314)
	require.ErrorContains(t, err, "is not tried again until then")
	assert.Equal(t, before, hits.Load(), "a remembered failure makes no request")
}

// A wheel answer that covers every platform is kept for good; one that does
// not is asked again after a week, since the missing wheel can be uploaded.
func TestWheelAnswersExpireUnlessComplete(t *testing.T) {
	hits := rootsServer(t, served(nil))
	ctx := context.Background()
	for _, dist := range []string{"statsd==4.0.1", "linuxonly==1.0"} {
		name, release, _ := strings.Cut(dist, "==")
		_, err := wheelsOf(ctx, name, release, "3.14")
		require.NoError(t, err)
	}
	dir := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", wheelsCacheDir)
	old := time.Now().Add(-8 * 24 * time.Hour)
	for _, f := range []string{"statsd==4.0.1-python-3.14", "linuxonly==1.0-python-3.14"} {
		require.NoError(t, os.Chtimes(filepath.Join(dir, f), old, old))
	}
	before := hits.Load()
	_, err := wheelsOf(ctx, "statsd", "4.0.1", "3.14")
	require.NoError(t, err)
	assert.Equal(t, before, hits.Load(), "a complete answer is kept for good")
	_, err = wheelsOf(ctx, "linuxonly", "1.0", "3.14")
	require.NoError(t, err)
	assert.Equal(t, before+1, hits.Load(), "a partial answer is asked again")
}

// The index the environment points uv or pip at is the one read, a mirror
// that serves only HTML pages included.
func TestWheelsAreReadFromTheConfiguredIndex(t *testing.T) {
	rootsServer(t, nil)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pypi/simple/statsd/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><a href="../../f/statsd-4.0.1-py2.py3-none-any.whl#sha256=00">statsd-4.0.1-py2.py3-none-any.whl</a></body></html>`))
	}))
	t.Cleanup(mirror.Close)
	t.Setenv("UV_DEFAULT_INDEX", mirror.URL+"/pypi/simple/")
	where, err := wheelsOf(context.Background(), "statsd", "4.0.1", "3.14")
	require.NoError(t, err)
	assert.Equal(t, everyPlatform, where)
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

// A wheel covers the platforms its tag says, on a Python its tags fit: py3,
// this CPython, or an older CPython's stable ABI; not another CPython, nor
// this one's free-threaded build, nor musl, 32-bit or Intel macOS. A release
// with no wheel at all is pure Python as far as can be told.
func TestWheelPlatforms(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  int
	}{
		{"pure Python", []string{"x-1.0-py3-none-any.whl"}, everyPlatform},
		{"sdist only", []string{"x-1.0.tar.gz"}, everyPlatform},
		{"Windows only", []string{"x-1.0-cp314-cp314-win_amd64.whl"}, 0},
		{"another version's wheel", []string{"x-0.9-cp314-cp314-manylinux_2_17_x86_64.whl", "x-1.0.tar.gz"}, everyPlatform},
		{"another CPython", []string{"x-1.0-cp313-cp313-manylinux_2_17_x86_64.whl"}, 0},
		{"free-threaded", []string{"x-1.0-cp314-cp314t-macosx_11_0_arm64.whl"}, 0},
		{"stable ABI", []string{"x-1.0-cp39-abi3-manylinux_2_17_aarch64.manylinux2014_aarch64.whl"}, linuxArm},
		{"Apple silicon", []string{"x-1.0-cp314-cp314-macosx_11_0_arm64.whl", "x-1.0-cp314-cp314-macosx_10_15_x86_64.whl"}, darwinArm},
		{"universal2", []string{"x-1.0-cp314-cp314-macosx_10_9_universal2.whl"}, darwinArm},
		{"musl only", []string{"x-1.0-cp314-cp314-musllinux_1_2_x86_64.whl"}, 0},
		{"32-bit Linux", []string{"x-1.0-cp314-cp314-manylinux_2_17_i686.whl"}, 0},
		{"build tag", []string{"x-1.0-1-cp314-cp314-manylinux_2_17_x86_64.whl"}, linuxX86},
		{"every platform", []string{"x-1.0-cp314-cp314-manylinux_2_17_x86_64.whl", "x-1.0-cp314-cp314-manylinux_2_17_aarch64.whl", "x-1.0-cp314-cp314-macosx_11_0_arm64.whl"}, everyPlatform},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, wheelPlatforms(tc.files, "1.0", "3.14"))
		})
	}
}

// A marker names exactly the platforms covered.
func TestPlatformMarker(t *testing.T) {
	assert.Equal(t, "sys_platform == 'darwin' and platform_machine == 'arm64'", platformMarker(darwinArm))
	assert.Equal(t, "(sys_platform == 'linux' and platform_machine == 'x86_64') or (sys_platform == 'darwin' and platform_machine == 'arm64')",
		platformMarker(linuxX86|darwinArm))
}
