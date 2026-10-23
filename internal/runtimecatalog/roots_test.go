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

// fakeUV answers `uv pip compile` without uv: answer gets the input, the
// platform, whether it is the annotated roots compile, and the packages the
// call holds to wheels. It counts the calls, and keeps every package any call
// held to wheels.
type fakeUV struct {
	calls, clients atomic.Int32
	answer         func(input []string, python, platform string, annotated bool, held []string) (string, error)
	mu             sync.Mutex
	held           []string
}

func (f *fakeUV) install(t *testing.T) {
	t.Helper()
	old := newCompiler
	newCompiler = func(context.Context) (compileFunc, error) {
		f.clients.Add(1)
		return f.compile, nil
	}
	t.Cleanup(func() { newCompiler = old })
}

func (f *fakeUV) compile(_ context.Context, _ string, input, args []string) (string, error) {
	f.calls.Add(1)
	python, platform, annotated := "", "", false
	var held []string
	for i, a := range args {
		switch a {
		case "--python-version":
			python = args[i+1]
		case "--python-platform":
			platform = args[i+1]
		case "--annotation-style":
			annotated = true
		case "--only-binary":
			held = append(held, args[i+1])
		}
	}
	f.mu.Lock()
	f.held = append(f.held, held...)
	f.mu.Unlock()
	return f.answer(input, python, platform, annotated, held)
}

// noWheel is uv's answer when --only-binary leaves dist no wheel.
func noWheel(dist string) error {
	return &uv.ResolutionError{Stderr: "× No solution found when resolving dependencies:\n  hint: Wheels are required for `" + dist + "` because building from source is disabled"}
}

var freeze339 = []string{
	"apache-airflow==3.3.2+astro.2", "apache-airflow-core==3.3.2+astro.2", "lazy-object-proxy==1.10.0",
	"apache-airflow-providers-mysql==6.4.0", "mysqlclient==2.2.8", "statsd==4.0.1", "linuxonly==1.0",
	"apache-airflow-providers-google==10.26.0", "json-merge-patch==0.2",
	"astronomer-kubernetes-executor==10.22.0+astro.1", "pip==26.2.1",
}

// answers339 is uv on freeze339: Airflow pulls in lazy-object-proxy (no macOS
// wheel, and exempt); providers-mysql pulls in mysqlclient on Linux, whose
// only wheels are Windows' (native); linuxonly has none for macOS;
// providers-google pulls in json-merge-patch, which publishes no wheel at all
// (pure Python, built from source).
func answers339(input []string, _, platform string, _ bool, held []string) (string, error) {
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
		if !strings.Contains(platform, "windows") {
			return "", noWheel("mysqlclient")
		}
	case "linuxonly":
		if !linux {
			return "", noWheel("linuxonly")
		}
	case "json-merge-patch":
		if slices.Contains(held, "json-merge-patch") {
			return "", noWheel("json-merge-patch")
		}
	case "apache-airflow-providers-google":
		if slices.Contains(held, "json-merge-patch") {
			return "", noWheel("json-merge-patch")
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
// providers-mysql only on macOS, since on Linux it pulls in mysqlclient, which
// is native (its wheels are Windows'); linuxonly only on Linux; mysqlclient
// left out with uv's reason. providers-google pulls in json-merge-patch, which
// publishes no wheel at all: taken for pure Python, it may build, and google
// is listed everywhere. What Airflow pulls in is never held to wheels.
func TestRuntimeRootsListsEachForThePlatformsItInstallsOn(t *testing.T) {
	rootsServer(t, map[string]string{published339: "apache-airflow-providers-mysql\nmysqlclient\nstatsd\nlinuxonly\npip\nastronomer-runtime-extensions\napache-airflow-providers-google\n"})
	f := &fakeUV{answer: answers339}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow-providers-google", "apache-airflow-providers-mysql; " + darwinMarker, "linuxonly; " + linuxMarker, "statsd"}, got.Names)
	assert.Equal(t, []LeftOut{{Name: "mysqlclient", Why: "it has no wheel on Linux x86_64, Linux aarch64, macOS arm64 (Python 3.14)"}}, got.LeftOut)
	assert.Equal(t, int32(1), f.clients.Load(), "one uv client for the whole run")
	assert.NotContains(t, f.held, "lazy-object-proxy", "what Airflow pulls in may build")
	assert.Contains(t, f.held, "mysqlclient")

	// Every answer is kept: a second call asks uv nothing, nor makes a client.
	before := f.calls.Load()
	again, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, got, again)
	assert.Equal(t, before, f.calls.Load())
	assert.Equal(t, int32(1), f.clients.Load())
}

// The reason names the package a root pulls in when that is what has no
// wheel.
func TestRuntimeRootsSaysWhatARootPullsIn(t *testing.T) {
	rootsServer(t, map[string]string{published339: "apache-airflow-providers-mysql\n"})
	f := &fakeUV{answer: func(input []string, _, platform string, _ bool, _ []string) (string, error) {
		if slices.Contains(input, "apache-airflow") {
			return "", nil
		}
		if input[0] == "mysqlclient" && strings.Contains(platform, "windows") {
			return "mysqlclient==2.2.8\n", nil
		}
		return "", noWheel("mysqlclient")
	}}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, []LeftOut{{
		Name: "apache-airflow-providers-mysql",
		Why:  "it pulls in mysqlclient, which has no wheel on Linux x86_64, Linux aarch64, macOS arm64 (Python 3.14)",
	}}, got.LeftOut)
}

// Without a published list, uv works the roots out: the candidates whose
// via-list names no other candidate and no Airflow distribution, kept.
func TestRuntimeRootsWorksOutTheList(t *testing.T) {
	rootsServer(t, map[string]string{})
	f := &fakeUV{answer: func(input []string, python, platform string, annotated bool, held []string) (string, error) {
		if annotated {
			return "apache-airflow==3.3.2+astro.2\n" +
				"lazy-object-proxy==1.10.0    # via apache-airflow, -c constraints.txt\n" +
				"mysqlclient==2.2.8           # via apache-airflow-providers-mysql, -c constraints.txt\n" +
				"apache-airflow-providers-mysql==6.4.0  # via -c constraints.txt\n" +
				"statsd==4.0.1                # via -c constraints.txt\n" +
				"linuxonly==1.0               # via linuxonly, -c constraints.txt\n" +
				"apache-airflow-providers-google==10.26.0  # via -c constraints.txt\n" +
				"json-merge-patch==0.2        # via apache-airflow-providers-google, -c constraints.txt\n", nil
		}
		return answers339(input, python, platform, false, held)
	}}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, []string{"apache-airflow-providers-google", "apache-airflow-providers-mysql; " + darwinMarker, "linuxonly; " + linuxMarker, "statsd"}, got.Names)
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", rootsComputedDir, "runtime-3.3-9-python-3.14.txt"))
	require.NoError(t, err)
	assert.Equal(t, "apache-airflow-providers-google\napache-airflow-providers-mysql\nlinuxonly\nstatsd\n", string(data))
}

// A failure to ask uv (the network, uv itself) fails the whole list, which
// keeps the group as it is, and is remembered for an hour; it is not kept as
// an answer.
func TestRuntimeRootsRemembersAFailure(t *testing.T) {
	rootsServer(t, map[string]string{published339: "statsd\n"})
	f := &fakeUV{answer: func([]string, string, string, bool, []string) (string, error) {
		return "", errors.New("dial tcp: no route to host")
	}}
	f.install(t)
	_, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.ErrorContains(t, err, "no route to host")
	before := f.calls.Load()
	_, err = RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.ErrorContains(t, err, "is not tried again until then")
	assert.Equal(t, before, f.calls.Load())
}

// ASTRO_RUNTIME_ROOTS_URL points the published lists at a mirror, read by the
// CDN's file names.
func TestRuntimeRootsReadsTheListFromAMirror(t *testing.T) {
	rootsServer(t, map[string]string{"/mirror/roots/runtime-3.3-9-python-3.14.txt": "statsd\n"})
	t.Setenv(RootsURLEnv, strings.TrimSuffix(rootsBaseURL, "/runtime-roots")+"/mirror/roots/")
	f := &fakeUV{answer: answers339}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, []string{"statsd"}, got.Names)
}

// uv 0.11 names the package with no wheel in its hint, and in the solver's
// own words, either of which is read.
func TestMissingWheel(t *testing.T) {
	both := "  × No solution found when resolving dependencies:\n  ╰─▶ Because mysqlclient==2.2.6 has no usable wheels and you require\n" +
		"      hint: Wheels are required for \x60mysqlclient\x60 because building from source"
	assert.Equal(t, "mysqlclient", missingWheel(both))
	assert.Equal(t, "json-merge-patch", missingWheel("Because json-merge-patch==0.2 has no usable wheels and you require it"))
	assert.Empty(t, missingWheel("Because you require foo>=2 and foo<2, we can conclude"))
}

// A canceled start is not a failure to remember.
func TestRuntimeRootsDoesNotRememberACancelledStart(t *testing.T) {
	rootsServer(t, map[string]string{published339: "statsd\n"})
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeUV{answer: func([]string, string, string, bool, []string) (string, error) { cancel(); return "", context.Canceled }}
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

// checkAnswer is what the cache holds for name, of 3.3-9 on Python 3.14.
func checkAnswer(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", rootsChecksDir, "3.3-9-python-3.14", name))
	require.NoError(t, err)
	return string(data)
}

// A package with wheels for an older Python but none for the project's is
// native, not built for this Python yet: the root that pulls it in stays out,
// and that answer expires, since the wheel can come. One uv refuses for
// another reason than a missing wheel is kept for an hour only; a missing
// wheel, for good.
func TestRuntimeRootsKeepsAnAnswerThatCanChangeForAWhile(t *testing.T) {
	rootsServer(t, map[string]string{published339: "oldbuilt-user\nweird\nmysqlclient\n"})
	f := &fakeUV{answer: func(input []string, python, platform string, _ bool, held []string) (string, error) {
		switch {
		case slices.Contains(input, "apache-airflow"):
			return "", nil
		case input[0] == "oldbuilt-user", input[0] == "oldbuilt" && python == "3.14":
			return "", noWheel("oldbuilt")
		case input[0] == "weird":
			return "", &uv.ResolutionError{Stderr: "× No solution found: weird requires something else"}
		}
		return answers339(input, python, platform, false, held)
	}}
	f.install(t)
	freeze := append(slices.Clone(freeze339), "oldbuilt-user==1.0", "oldbuilt==1.0", "weird==1.0")
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze)
	require.NoError(t, err)
	assert.Empty(t, got.Names)
	assert.Contains(t, checkAnswer(t, "oldbuilt-user@aarch64-apple-darwin"), "expires ")
	assert.Contains(t, checkAnswer(t, "source-only@oldbuilt"), "later\nexpires ")
	assert.Contains(t, checkAnswer(t, "weird@aarch64-apple-darwin"), "expires ")
	assert.NotContains(t, checkAnswer(t, "mysqlclient@aarch64-apple-darwin"), "expires ")
}

// The budget running out while the caller still waits is a failure to
// remember, as the network failing is: a blackholed network costs one wait an
// hour.
func TestRuntimeRootsRemembersARunOutBudget(t *testing.T) {
	rootsServer(t, map[string]string{published339: "statsd\n"})
	f := &fakeUV{answer: func([]string, string, string, bool, []string) (string, error) { return "", context.DeadlineExceeded }}
	f.install(t)
	_, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "astro", rootsFailedDir, "runtime-3.3-9-python-3.14.txt"))
	assert.NoError(t, err)
}
