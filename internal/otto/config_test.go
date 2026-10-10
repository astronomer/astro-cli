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
	"github.com/astronomer/astro-cli/pkg/localrt"
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

// Inside a project that names its workspace's organization, Otto runs in
// that organization; one that names none, and no project at all, keep the
// login's.
func (s *ConfigSuite) TestNewConfigFromContextTakesTheProjectsOrganization() {
	s.Equal("test-org-id", NewConfigFromContext().Organization, "outside a project")

	dir := s.chdirTempProject("org-project")
	body := "[project]\nname = \"p\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\nworkspace = \"cmws\"\n"
	s.Require().NoError(os.WriteFile(filepath.Join(dir, project.Marker), []byte(body), 0o600))
	s.Equal("test-org-id", NewConfigFromContext().Organization, "a project naming no organization")

	s.Require().NoError(os.WriteFile(filepath.Join(dir, project.Marker), []byte(body+"organization = \"clother\"\n"), 0o600))
	s.Equal("clother", NewConfigFromContext().Organization)
	env := strings.Join((&Config{Organization: NewConfigFromContext().Organization}).BuildEnv(), "\n")
	s.Contains(env, "ASTRO_ORGANIZATION=clother")
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

func (s *ConfigSuite) TestDetectAirflow_ProjectProjectHealthy() {
	srv := s.startFakeAirflow()
	defer srv.Close()

	cwd := s.chdirManifestProject("project-healthy")
	s.writeProjectRecord(cwd, serverPort(srv))

	s.Equal(fmt.Sprintf("http://localhost:%d", serverPort(srv)), detectedURL())
}

func (s *ConfigSuite) TestDetectAirflow_ProjectStaleRecord() {
	// A record whose Airflow is gone fails the health probe and must yield
	// nothing rather than a dead URL.
	cwd := s.chdirManifestProject("project-stale")
	s.writeProjectRecord(cwd, unusedPort(s.T()))

	s.Empty(detectedURL())
}

func (s *ConfigSuite) TestDetectAirflow_ProjectAirflow2StandaloneRefused() {
	// A standalone Airflow 2 record means a generated admin password otto can't
	// read — detection must yield nothing rather than a URL with wrong
	// credentials.
	srv := s.startFakeAirflow()
	defer srv.Close()

	cwd := s.chdirManifestProject("project-airflow2")
	s.Require().NoError(localrttest.Seed(localrttest.Record{
		ProjectPath:  cwd,
		Port:         serverPort(srv),
		Mode:         localrt.ModeStandalone,
		AirflowMajor: "2",
	}))

	s.Empty(detectedURL())
}

func (s *ConfigSuite) TestDetectAirflow_ProjectAirflow2DockerDetected() {
	// Docker mode creates admin/admin itself, which is the pair BuildEnv sends,
	// so an Airflow 2 project running in containers is reachable.
	srv := s.startFakeAirflow()
	defer srv.Close()

	cwd := s.chdirManifestProject("project-airflow2-docker")
	s.Require().NoError(localrttest.Seed(localrttest.Record{
		ProjectPath:  cwd,
		Port:         serverPort(srv),
		Mode:         localrt.ModeDocker,
		AirflowMajor: "2",
	}))

	s.Equal(fmt.Sprintf("http://localhost:%d", serverPort(srv)), detectedURL())
}

func (s *ConfigSuite) TestDetectAirflow_ProjectProjectNoRecord() {
	s.chdirManifestProject("project-not-started")

	s.Empty(detectedURL())
}

func (s *ConfigSuite) TestDetectAirflow_ProjectWinsOver1xRoute() {
	// A directory can carry both a project's state record and a stale 1.x route (a
	// project migrated in place). The project's own Airflow wins.
	projectSrv := s.startFakeAirflow()
	defer projectSrv.Close()
	routeSrv := s.startFakeAirflow()
	defer routeSrv.Close()

	cwd := s.chdirManifestProject("project-migrated")
	s.writeProjectRecord(cwd, serverPort(projectSrv))
	s.writeRoute(&pkgproxy.Route{
		Hostname:   "project-migrated.localhost",
		Port:       urlPort(s.T(), routeSrv.URL),
		ProjectDir: cwd,
		PID:        os.Getpid(),
	})

	s.Equal(fmt.Sprintf("http://localhost:%d", serverPort(projectSrv)), detectedURL())
}

func (s *ConfigSuite) TestDetectAirflow_NoRouteForProject() {
	// With a fresh tmpDir HomeConfigPath, there is no routes.json at all.
	// Previously DetectAirflow would fall through to probing the home-global
	// WebserverPort (8080), which could silently match another project's
	// Airflow. It must now return empty instead.
	s.chdirTempProject("no-route-project")

	s.Empty(detectedURL())
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

	s.Equal(fmt.Sprintf("http://localhost:%s", port), detectedURL())
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

	s.Empty(detectedURL())
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
	s.Empty(detectedURL())
}

// --- helpers ---

func (s *ConfigSuite) chdirTempProject(name string) string {
	dir := filepath.Join(s.T().TempDir(), name)
	s.Require().NoError(os.MkdirAll(dir, 0o755))
	orig, err := os.Getwd()
	s.Require().NoError(err)
	s.Require().NoError(os.Chdir(dir))
	s.T().Cleanup(func() { _ = os.Chdir(orig) })
	// What os.Getwd() inside DetectAirflow observes, which is not dir itself:
	// macOS renders /var/folders via a /private/var symlink, and Windows keeps
	// the 8.3 short name (RUNNER~1) that filepath.EvalSymlinks would expand.
	cwd, err := os.Getwd()
	s.Require().NoError(err)
	return cwd
}

// chdirManifestProject is chdirTempProject plus a project manifest. Discovery keys
// on the file's presence alone; the [tool.astro] body just makes the fixture
// look like a real project.
func (s *ConfigSuite) chdirManifestProject(name string) string {
	dir := s.chdirTempProject(name)
	manifest := "[tool.astro]\n"
	s.Require().NoError(os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600))
	return dir
}

// writeProjectRecord writes the smallest record detection reads: path and port.
func (s *ConfigSuite) writeProjectRecord(projectPath string, port int) {
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
