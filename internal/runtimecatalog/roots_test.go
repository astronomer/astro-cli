package runtimecatalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/uv"
)

// fakeUV answers `uv pip compile` without uv: answer gets the input and the
// platform, and whether it is the annotated roots compile. It counts the
// calls, and fails the test when one holds an exempt package to wheels.
type fakeUV struct {
	calls  atomic.Int32
	answer func(input []string, platform string, annotated bool) (string, error)
	mu     sync.Mutex
	held   []string
}

func (f *fakeUV) install(t *testing.T) {
	t.Helper()
	old := pipCompile
	pipCompile = func(_ context.Context, _ string, input, args []string) (string, error) {
		f.calls.Add(1)
		platform, annotated := "", false
		for i, a := range args {
			switch a {
			case "--python-platform":
				platform = args[i+1]
			case "--annotation-style":
				annotated = true
			case "--only-binary":
				f.mu.Lock()
				f.held = append(f.held, args[i+1])
				f.mu.Unlock()
			}
		}
		return f.answer(input, platform, annotated)
	}
	t.Cleanup(func() { pipCompile = old })
}

// noWheel is uv's answer when --only-binary leaves dist no wheel.
func noWheel(dist string) error {
	return &uv.ResolutionError{Stderr: "× No solution found when resolving dependencies:\n  hint: Wheels are required for `" + dist + "` because building from source is disabled"}
}

var freeze339 = []string{
	"apache-airflow==3.3.2+astro.2", "apache-airflow-core==3.3.2+astro.2", "lazy-object-proxy==1.10.0",
	"apache-airflow-providers-mysql==6.4.0", "mysqlclient==2.2.8", "statsd==4.0.1", "linuxonly==1.0",
	"astronomer-kubernetes-executor==10.22.0+astro.1", "pip==26.2.1",
}

// answers339 is uv on freeze339: Airflow pulls in lazy-object-proxy (no macOS
// wheel, and exempt); providers-mysql pulls in mysqlclient on Linux, which has
// no wheel anywhere; linuxonly has none for macOS.
func answers339(input []string, platform string, _ bool) (string, error) {
	if slices.Contains(input, "apache-airflow") {
		return "apache-airflow==3.3.2+astro.2\napache-airflow-core==3.3.2+astro.2\nlazy-object-proxy==1.10.0\n", nil
	}
	linux := strings.Contains(platform, "manylinux")
	switch input[0] {
	case "apache-airflow-providers-mysql":
		if linux {
			return "", noWheel("mysqlclient")
		}
	case "mysqlclient":
		return "", noWheel("mysqlclient")
	case "linuxonly":
		if !linux {
			return "", noWheel("linuxonly")
		}
	}
	return input[0] + "==1.0\n", nil
}

const darwinMarker = "sys_platform == 'darwin' and platform_machine == 'arm64'"

const linuxMarker = "(sys_platform == 'linux' and platform_machine == 'x86_64') or (sys_platform == 'linux' and platform_machine == 'aarch64')"

// rootsServer points the cache and the CDN at test-owned places, serving
// files.
func rootsServer(t *testing.T, files map[string]string) {
	t.Helper()
	srv, _ := freezeServer(t, files)
	old := rootsBaseURL
	rootsBaseURL = srv.URL + "/runtime-roots"
	t.Cleanup(func() { rootsBaseURL = old })
}

const published339 = "/runtime-roots/runtime-3.3-9-python-3.14.txt"

// Each root is listed for the platforms uv installs it on from wheels alone:
// providers-mysql only on macOS, since on Linux it pulls in mysqlclient;
// linuxonly only on Linux; mysqlclient, with no wheel anywhere, left out with
// uv's reason. What Airflow pulls in is never held to wheels.
func TestRuntimeRootsListsEachForThePlatformsItInstallsOn(t *testing.T) {
	rootsServer(t, map[string]string{published339: "apache-airflow-providers-mysql\nmysqlclient\nstatsd\nlinuxonly\npip\nastronomer-runtime-extensions\n"})
	f := &fakeUV{answer: answers339}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow-providers-mysql; " + darwinMarker, "linuxonly; " + linuxMarker, "statsd"}, got.Names)
	assert.Equal(t, []LeftOut{{Name: "mysqlclient", Why: "it has no wheel on Linux x86_64, Linux aarch64, macOS arm64 (Python 3.14)"}}, got.LeftOut)
	assert.NotContains(t, f.held, "lazy-object-proxy", "what Airflow pulls in may build")
	assert.Contains(t, f.held, "mysqlclient")

	// Every answer is kept: a second call asks uv nothing.
	before := f.calls.Load()
	again, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, got, again)
	assert.Equal(t, before, f.calls.Load())
}

// The reason names the package a root pulls in when that is what has no
// wheel.
func TestRuntimeRootsSaysWhatARootPullsIn(t *testing.T) {
	rootsServer(t, map[string]string{published339: "apache-airflow-providers-mysql\n"})
	f := &fakeUV{answer: func(input []string, platform string, _ bool) (string, error) {
		if slices.Contains(input, "apache-airflow") {
			return "", nil
		}
		return "", noWheel("mysqlclient")
	}}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, []LeftOut{{Name: "apache-airflow-providers-mysql",
		Why: "it pulls in mysqlclient, which has no wheel on Linux x86_64, Linux aarch64, macOS arm64 (Python 3.14)"}}, got.LeftOut)
}

// Without a published list, uv works the roots out: the candidates whose
// via-list names no other candidate and no Airflow distribution, kept.
func TestRuntimeRootsWorksOutTheList(t *testing.T) {
	rootsServer(t, map[string]string{})
	f := &fakeUV{answer: func(input []string, platform string, annotated bool) (string, error) {
		if annotated {
			return "apache-airflow==3.3.2+astro.2\n" +
				"lazy-object-proxy==1.10.0    # via apache-airflow, -c constraints.txt\n" +
				"mysqlclient==2.2.8           # via apache-airflow-providers-mysql, -c constraints.txt\n" +
				"apache-airflow-providers-mysql==6.4.0  # via -c constraints.txt\n" +
				"statsd==4.0.1                # via -c constraints.txt\n" +
				"linuxonly==1.0               # via linuxonly, -c constraints.txt\n", nil
		}
		return answers339(input, platform, false)
	}}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow-providers-mysql; " + darwinMarker, "linuxonly; " + linuxMarker, "statsd"}, got.Names)
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", rootsComputedDir, "runtime-3.3-9-python-3.14.txt"))
	require.NoError(t, err)
	assert.Equal(t, "apache-airflow-providers-mysql\nlinuxonly\nstatsd\n", string(data))
}

// A failure to ask uv (the network, uv itself) fails the whole list, which
// keeps the group as it is, and is remembered for an hour; it is not kept as
// an answer.
func TestRuntimeRootsRemembersAFailure(t *testing.T) {
	rootsServer(t, map[string]string{published339: "statsd\n"})
	f := &fakeUV{answer: func([]string, string, bool) (string, error) { return "", errors.New("dial tcp: no route to host") }}
	f.install(t)
	_, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.ErrorContains(t, err, "no route to host")
	before := f.calls.Load()
	_, err = RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.ErrorContains(t, err, "is not tried again until then")
	assert.Equal(t, before, f.calls.Load())
}

// A cancelled start is not a failure to remember.
func TestRuntimeRootsDoesNotRememberACancelledStart(t *testing.T) {
	rootsServer(t, map[string]string{published339: "statsd\n"})
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeUV{answer: func([]string, string, bool) (string, error) { cancel(); return "", context.Canceled }}
	f.install(t)
	_, err := RuntimeRoots(ctx, "3.3-9", "3.14", freeze339)
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", rootsFailedDir, "runtime-3.3-9-python-3.14.txt"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// The candidates are the freeze less Airflow, pip, Astronomer's platform
// packages and its other +astro builds; astronomer-providers, which DAGs
// import, stays.
func TestRootsCandidates(t *testing.T) {
	candidates, airflow := rootsCandidates([]string{
		"apache-airflow==3.3.2+astro.2", "apache-airflow-task-sdk==1.3.2+astro.1", "pandas==2.3.3",
		"astronomer-kubernetes-executor==10.22.0+astro.1", "astronomer-fab-security-manager==1.10.0", "pip==26.2.1",
		"something==1.0+astro.1", "Statsd==4.0.1", "astronomer-providers==1.19.4",
	})
	assert.Equal(t, []string{"astronomer-providers", "pandas", "statsd"}, candidates)
	assert.Equal(t, []string{"apache-airflow", "apache-airflow-task-sdk"}, airflow)
}
