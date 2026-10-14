//go:build !windows

package proxy

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// fakeV1PortEnv makes the test binary act as an astro 1.x proxy on the port it
// names, so a test can stop a real process that matches the way a 1.x one runs.
const fakeV1PortEnv = "ASTRO_TEST_FAKE_V1_PROXY_PORT"

// realStopProcess is stopProcess before TestMain replaces it.
var realStopProcess = stopProcess

// TestMain keeps the tests off real processes. A developer's machine may run a
// real 1.x proxy on 6563, and the tests call EnsureRunning with that port; a
// test that wants real processes puts the real seams back itself.
func TestMain(m *testing.M) {
	if port := os.Getenv(fakeV1PortEnv); port != "" {
		serveFakeV1Proxy(port)
		return
	}
	listOwnProcesses = func() []process { return nil }
	stopProcess = func(int) {}
	os.Exit(m.Run())
}

func serveFakeV1Proxy(port string) {
	l, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		os.Exit(1)
	}
	srv := &http.Server{Handler: v1NotFound(), ReadHeaderTimeout: time.Second}
	go srv.Serve(l)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig
}

func v1NotFound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "<html><body>"+notFoundHeading+"</body></html>")
	})
}

// setupAstroHome points the route store at <astroHome>/.astro/proxy, where a
// 1.x CLI run with ASTRO_HOME=<astroHome> keeps its routes.
func setupAstroHome(t *testing.T) string {
	t.Helper()
	astroHome := t.TempDir()
	orig := config.HomeConfigPath
	config.HomeConfigPath = filepath.Join(astroHome, config.ConfigDir)
	t.Cleanup(func() { config.HomeConfigPath = orig })
	return astroHome
}

// listen holds a free loopback port with handler and returns the port.
func listen(t *testing.T, handler http.Handler) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
}

func fakeProcesses(t *testing.T, procs ...process) *[]int {
	t.Helper()
	var stopped []int
	origList, origStop := listOwnProcesses, stopProcess
	listOwnProcesses = func() []process { return procs }
	stopProcess = func(pid int) { stopped = append(stopped, pid) }
	t.Cleanup(func() { listOwnProcesses, stopProcess = origList, origStop })
	return &stopped
}

func v1Proxy(pid int, port, astroHome string) process {
	return process{
		pid:  pid,
		args: []string{"/opt/homebrew/bin/astro", "dev", "proxy", "serve", "--port", port},
		env:  []string{"HOME=/nowhere", "ASTRO_HOME=" + astroHome},
	}
}

func TestTakeOverFromV1(t *testing.T) {
	astroHome := setupAstroHome(t)
	v1Port := listen(t, v1NotFound())
	otherPort := listen(t, http.NotFoundHandler())

	for _, tc := range []struct {
		name  string
		port  string
		procs []process
		want  []int
	}{
		{"stops the 1.x proxy serving this store on the port", v1Port, []process{v1Proxy(41, v1Port, astroHome)}, []int{41}},
		{"finds the store through HOME when ASTRO_HOME is unset", v1Port, []process{{
			pid:  42,
			args: []string{"astro", "dev", "proxy", "serve", "--port=" + v1Port},
			env:  []string{"HOME=" + astroHome},
		}}, []int{42}},
		{"leaves a 1.x proxy that serves another store", v1Port, []process{v1Proxy(43, v1Port, t.TempDir())}, nil},
		{"leaves a 1.x proxy whose home is relative", v1Port, []process{v1Proxy(49, v1Port, "relative/home")}, nil},
		{"leaves a 1.x proxy for another port", v1Port, []process{v1Proxy(44, "6563", astroHome)}, nil},
		{"leaves this CLI's own daemon", v1Port, []process{{
			pid:  45,
			args: []string{"astro", ServeSubcommand, "--port", v1Port},
			env:  []string{"ASTRO_HOME=" + astroHome},
		}}, nil},
		{"leaves both when two claim the port", v1Port, []process{v1Proxy(46, v1Port, astroHome), v1Proxy(47, v1Port, astroHome)}, nil},
		{"leaves it when the port does not answer as an astro proxy", otherPort, []process{v1Proxy(48, otherPort, astroHome)}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stopped := fakeProcesses(t, tc.procs...)
			takeOverFromV1(tc.port)
			assert.Equal(t, tc.want, *stopped)
		})
	}
}

func TestTakeOverFromV1SkipsAFreePort(t *testing.T) {
	astroHome := setupAstroHome(t)
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
	require.NoError(t, l.Close())

	stopped := fakeProcesses(t, v1Proxy(41, port, astroHome))
	takeOverFromV1(port)
	assert.Empty(t, *stopped)
}

func TestV1ProxyPort(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"astro", "dev", "proxy", "serve", "--port", "6563"}, "6563"},
		{[]string{"astro", "dev", "proxy", "serve", "--port=7000"}, "7000"},
		{[]string{"astro", "dev", "proxy", "serve"}, pkgproxy.DefaultPort},
		{[]string{"astro", "--verbosity", "debug", "dev", "proxy", "serve", "--port", "6563"}, "6563"},
		{[]string{"astro", ServeSubcommand, "--port", "6563"}, ""},
		{[]string{"astro", "dev", "proxy", "status"}, ""},
		{[]string{"dev", "proxy", "serve"}, ""},
		{nil, ""},
	} {
		assert.Equal(t, tc.want, v1ProxyPort(tc.args), "%q", tc.args)
	}
}

// A real process that runs the way a 1.x proxy runs holds a port; what the
// daemon runs before starting on that port stops it and finds the port free.
func TestBeforeStartTakesThePortFromARunningV1Proxy(t *testing.T) {
	astroHome := setupAstroHome(t)
	origList, origStop := listOwnProcesses, stopProcess
	listOwnProcesses, stopProcess = ownProcesses, realStopProcess
	t.Cleanup(func() { listOwnProcesses, stopProcess = origList, origStop })

	l, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
	require.NoError(t, l.Close())

	fake := exec.Command(os.Args[0], "dev", "proxy", "serve", "--port", port)
	fake.Env = append(os.Environ(), fakeV1PortEnv+"="+port, "ASTRO_HOME="+astroHome)
	require.NoError(t, fake.Start())
	exited := make(chan struct{})
	go func() { fake.Wait(); close(exited) }()
	t.Cleanup(func() { fake.Process.Kill() })
	require.Eventually(t, func() bool { return !pkgproxy.IsPortAvailable(port) }, 5*time.Second, 20*time.Millisecond)

	// The daemon's own BeforeStart, which is what pkg/proxy runs before a
	// start; that it runs there, and before the start, is pkg/proxy's test.
	daemon().BeforeStart(port)

	assert.True(t, pkgproxy.IsPortAvailable(port), "the 1.x proxy should be gone before the daemon starts")
	select {
	case <-exited:
	case <-time.After(stopTimeout):
		t.Fatal("the 1.x proxy is still running")
	}
}
