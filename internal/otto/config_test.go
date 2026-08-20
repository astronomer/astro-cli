package otto

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt/localrttest"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type ConfigSuite struct {
	suite.Suite
	origHomeConfigPath string
	tmpDir             string
}

func (s *ConfigSuite) SetupTest() {
	s.origHomeConfigPath = config.HomeConfigPath
	s.tmpDir = s.T().TempDir()
	// InitTestConfig → config.InitConfig → initHome resets HomeConfigPath back
	// to the real ~/.astro, so the override has to come after, or route writes
	// leak into the user's real routes.json and the running proxy daemon picks
	// them up.
	testUtil.InitTestConfig(config.CloudPlatform)
	config.HomeConfigPath = s.tmpDir
	// localstate lives under XDG_CACHE_HOME — isolate it so a real running
	// Airflow's record can't leak into these tests.
	s.T().Setenv("XDG_CACHE_HOME", s.T().TempDir())
}

func (s *ConfigSuite) TearDownTest() {
	config.HomeConfigPath = s.origHomeConfigPath
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

func (s *ConfigSuite) TestNewConfigFromContext() {
	// With test config initialized, should get auth fields
	cfg := NewConfigFromContext()
	// Test config has a domain and token set
	s.NotEmpty(cfg.Domain)
}

func (s *ConfigSuite) TestBuildEnv_SetsVars() {
	cfg := &Config{
		Token:        "test-token",
		Domain:       "astronomer.io",
		Organization: "org-123",
		AirflowURL:   "http://localhost:8080",
	}

	env := cfg.BuildEnv()

	findEnv := func(key string) string {
		prefix := key + "="
		for _, e := range env {
			if strings.HasPrefix(e, prefix) {
				return strings.TrimPrefix(e, prefix)
			}
		}
		return ""
	}

	s.Equal("test-token", findEnv("ASTRO_TOKEN"))
	s.Equal("astronomer.io", findEnv("ASTRO_DOMAIN"))
	s.Equal("org-123", findEnv("ASTRO_ORGANIZATION"))
	s.Equal("http://localhost:8080", findEnv("AIRFLOW_API_URL"))
	s.Equal("admin", findEnv("AIRFLOW_USERNAME"))
	s.Equal("admin", findEnv("AIRFLOW_PASSWORD"))
	// Any `astro dev start` the agent runs should NOT open a browser — that's
	// a surprise focus-grab while the user is mid-conversation in the TUI.
	s.Equal("1", findEnv("ASTRONOMER_NO_BROWSER"))
}

func (s *ConfigSuite) TestBuildEnv_SkipsEmptyValues() {
	cfg := &Config{
		Token: "test-token",
		// AirflowURL is empty
	}

	env := cfg.BuildEnv()

	findEnv := func(key string) (string, bool) {
		prefix := key + "="
		for _, e := range env {
			if strings.HasPrefix(e, prefix) {
				return strings.TrimPrefix(e, prefix), true
			}
		}
		return "", false
	}
	hasKey := func(key string) bool { _, ok := findEnv(key); return ok }

	s.True(hasKey("ASTRO_TOKEN"))
	s.False(hasKey("AIRFLOW_API_URL"))
	s.False(hasKey("AIRFLOW_USERNAME"))
	s.False(hasKey("AIRFLOW_PASSWORD"))

	// When we couldn't associate an Airflow with the current project, we point
	// the af CLI at an empty config so it doesn't fall through to
	// ~/.af/config.yaml and silently query a different project's Airflow.
	afConfig, ok := findEnv("AF_CONFIG")
	s.True(ok, "AF_CONFIG should be set when AirflowURL is empty")
	s.Equal(os.DevNull, afConfig)
}

func (s *ConfigSuite) TestBuildEnv_DoesNotSetAFConfigWhenAirflowDetected() {
	cfg := &Config{AirflowURL: "http://localhost:14955"}
	env := cfg.BuildEnv()

	for _, e := range env {
		s.False(strings.HasPrefix(e, "AF_CONFIG="), "AF_CONFIG must not be overridden when Airflow is detected")
	}
}

func (s *ConfigSuite) TestBuildEnv_OverridesExisting() {
	os.Setenv("ASTRO_TOKEN", "old-token")
	defer os.Unsetenv("ASTRO_TOKEN")

	cfg := &Config{Token: "new-token"}
	env := cfg.BuildEnv()

	prefix := "ASTRO_TOKEN="
	count := 0
	value := ""
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			count++
			value = strings.TrimPrefix(e, prefix)
		}
	}

	s.Equal(1, count, "should have exactly one ASTRO_TOKEN entry")
	s.Equal("new-token", value)
}

func (s *ConfigSuite) TestDetectAirflow_V2ProjectHealthy() {
	srv := s.startFakeAirflow()
	defer srv.Close()

	cwd := s.chdirV2Project("v2-healthy")
	s.writeV2Record(cwd, serverPort(srv))

	s.Equal(fmt.Sprintf("http://localhost:%d", serverPort(srv)), DetectAirflow())
}

func (s *ConfigSuite) TestDetectAirflow_V2StaleRecord() {
	// A record whose Airflow is gone fails the health probe and must yield
	// nothing rather than a dead URL.
	cwd := s.chdirV2Project("v2-stale")
	s.writeV2Record(cwd, unusedPort(s.T()))

	s.Empty(DetectAirflow())
}

func (s *ConfigSuite) TestDetectAirflow_V2Airflow2Refused() {
	// An Airflow 2 record means a generated admin password otto can't read —
	// detection must yield nothing rather than a URL with wrong credentials.
	srv := s.startFakeAirflow()
	defer srv.Close()

	cwd := s.chdirV2Project("v2-airflow2")
	s.Require().NoError(localrttest.Seed(localrttest.Record{
		ProjectPath:  cwd,
		Port:         serverPort(srv),
		AirflowMajor: "2",
	}))

	s.Empty(DetectAirflow())
}

func (s *ConfigSuite) TestDetectAirflow_V2ProjectNoRecord() {
	s.chdirV2Project("v2-not-started")

	s.Empty(DetectAirflow())
}

func (s *ConfigSuite) TestDetectAirflow_V2WinsOverV1Route() {
	// A directory can carry both a v2 state record and a stale v1 route (a
	// project migrated in place). The v2 project's own Airflow wins.
	v2srv := s.startFakeAirflow()
	defer v2srv.Close()
	v1srv := s.startFakeAirflow()
	defer v1srv.Close()

	cwd := s.chdirV2Project("v2-migrated")
	s.writeV2Record(cwd, serverPort(v2srv))
	s.writeRoute(&pkgproxy.Route{
		Hostname:   "v2-migrated.localhost",
		Port:       urlPort(s.T(), v1srv.URL),
		ProjectDir: cwd,
		PID:        os.Getpid(),
	})

	s.Equal(fmt.Sprintf("http://localhost:%d", serverPort(v2srv)), DetectAirflow())
}

func (s *ConfigSuite) TestDetectAirflow_NoRouteForProject() {
	// With a fresh tmpDir HomeConfigPath, there is no routes.json at all.
	// Previously DetectAirflow would fall through to probing the home-global
	// WebserverPort (8080), which could silently match another project's
	// Airflow. It must now return empty instead.
	s.chdirTempProject("no-route-project")

	s.Empty(DetectAirflow())
}

func (s *ConfigSuite) TestDetectAirflow_RouteExistsAndHealthy() {
	// Spin up a fake Airflow that answers the health endpoint, write a route
	// pointing the CWD to its port, and verify DetectAirflow picks it up.
	srv := s.startFakeAirflow()
	defer srv.Close()

	port := urlPort(s.T(), srv.URL)
	cwd := s.chdirTempProject("healthy-project")
	s.writeRoute(&pkgproxy.Route{
		Hostname:   "healthy-project.localhost",
		Port:       port,
		ProjectDir: cwd,
		PID:        os.Getpid(),
	})

	s.Equal(fmt.Sprintf("http://localhost:%s", port), DetectAirflow())
}

func (s *ConfigSuite) TestDetectAirflow_RouteExistsButUnhealthy() {
	// Route is registered for this project but the port doesn't answer a
	// health check (e.g. the standalone PID died without cleaning the route).
	// We must not return the URL — callers would fail confusingly against it.
	cwd := s.chdirTempProject("unhealthy-project")
	s.writeRoute(&pkgproxy.Route{
		Hostname:   "unhealthy-project.localhost",
		Port:       fmt.Sprint(unusedPort(s.T())),
		ProjectDir: cwd,
		PID:        os.Getpid(),
	})

	s.Empty(DetectAirflow())
}

func (s *ConfigSuite) TestDetectAirflow_IgnoresOtherProjectsRoutes() {
	// Another project is healthy on routes.json, but it's not *this* project's
	// directory. The old behavior would fall back to the home-global webserver
	// port and match it anyway; the new behavior must return empty.
	srv := s.startFakeAirflow()
	defer srv.Close()

	port := urlPort(s.T(), srv.URL)
	otherDir := filepath.Join(s.T().TempDir(), "other-project")
	s.Require().NoError(os.MkdirAll(otherDir, 0o755))
	s.writeRoute(&pkgproxy.Route{
		Hostname:   "other-project.localhost",
		Port:       port,
		ProjectDir: otherDir,
		PID:        os.Getpid(),
	})

	s.chdirTempProject("current-project")
	s.Empty(DetectAirflow())
}

// --- helpers ---

func (s *ConfigSuite) chdirTempProject(name string) string {
	dir := filepath.Join(s.T().TempDir(), name)
	s.Require().NoError(os.MkdirAll(dir, 0o755))
	orig, err := os.Getwd()
	s.Require().NoError(err)
	s.Require().NoError(os.Chdir(dir))
	s.T().Cleanup(func() { _ = os.Chdir(orig) })
	// Resolve symlinks so the path matches what os.Getwd() inside DetectAirflow
	// observes (macOS renders /var/folders via a /private/var symlink).
	resolved, err := filepath.EvalSymlinks(dir)
	s.Require().NoError(err)
	return resolved
}

// chdirV2Project is chdirTempProject plus a project manifest. Discovery keys
// on the file's presence alone; the [tool.astro] body just makes the fixture
// look like a real project.
func (s *ConfigSuite) chdirV2Project(name string) string {
	dir := s.chdirTempProject(name)
	manifest := "[tool.astro]\nairflow = '3.1'\n"
	s.Require().NoError(os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600))
	return dir
}

// writeV2Record writes the smallest record detection reads: path and port.
func (s *ConfigSuite) writeV2Record(projectPath string, port int) {
	s.T().Helper()
	s.Require().NoError(localrttest.Seed(localrttest.Record{
		ProjectPath: projectPath,
		Port:        port,
	}))
}

func (s *ConfigSuite) writeRoute(r *pkgproxy.Route) {
	s.T().Helper()
	s.Require().NoError(proxy.Routes().AddRoute(r))
}

func (s *ConfigSuite) startFakeAirflow() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/monitor/health" || r.URL.Path == "/api/v1/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func urlPort(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parsing %q: %v", rawURL, err)
	}
	return u.Port()
}

func serverPort(srv *httptest.Server) int {
	return srv.Listener.Addr().(*net.TCPAddr).Port
}

func unusedPort(t *testing.T) int {
	t.Helper()
	// Listen on :0 to grab a free port, then close the listener so the port
	// is unused by the time the caller tries to health-check it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}
