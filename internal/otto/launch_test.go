package otto

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/logger"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// errPrivilegeNotHeld is Windows' ERROR_PRIVILEGE_NOT_HELD, what os.Symlink
// returns there for an account without the symlink privilege. A plain Errno so
// the test compiles everywhere; the check is gated on GOOS besides.
const errPrivilegeNotHeld = syscall.Errno(1314)

func init() {
	// The vault's master key is keyed by service name, not path: without the
	// mock a launch test would reach the developer's real keychain.
	keyring.MockInit()
}

// launch is one `astro otto` run through Start with the Otto spawn stubbed:
// what it would have been handed, and where the warehouse files went.
type launch struct {
	bin  string
	args []string
	env  []string
}

func (l launch) get(key string) (string, bool) {
	prefix := key + "="
	for _, e := range l.env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}

// prepareLaunch makes Start runnable offline: an installed Otto that is
// current, an update cache that is fresh, the vault and the skill's config dir
// under temp directories, and the spawn replaced. It returns the warehouse
// config dir and the vault dir.
func (s *ConfigSuite) prepareLaunch() (warehouses, vault string) {
	s.T().Helper()
	home := s.T().TempDir()
	s.T().Setenv("HOME", home)
	s.T().Setenv("USERPROFILE", home)
	vault, err := secrets.DefaultDir()
	s.Require().NoError(err)

	s.Require().NoError(os.MkdirAll(BinDir(), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(BinDir(), pkgJSON), []byte(`{"version":"99.0.0"}`), 0o600))
	state, err := json.Marshal(updateState{LastCheck: time.Now().UTC().Format(time.RFC3339), LatestKnown: "99.0.0", Channel: Channel()})
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(filepath.Join(BinDir(), updateStateFile), state, 0o600))

	warehouses = filepath.Join(s.T().TempDir(), "agents")
	origDir, origSpawn := warehouseDir, spawnOtto
	warehouseDir = func() (string, error) { return warehouses, nil }
	s.T().Cleanup(func() {
		warehouseDir, spawnOtto = origDir, origSpawn
		logger.SetOutput(os.Stderr) // Start points the logger at a file it closes
	})
	return warehouses, vault
}

// start runs Start with the spawn captured.
func (s *ConfigSuite) start(args ...string) launch {
	s.T().Helper()
	var got launch
	spawned := false
	spawnOtto = func(bin string, args, env []string) error {
		spawned = true
		got = launch{bin: bin, args: args, env: env}
		return nil
	}
	s.Require().NoError(Start(args))
	s.Require().True(spawned, "Start never reached the Otto spawn")
	return got
}

// putConn stores a connection in the shared vault the way both tools do.
func (s *ConfigSuite) putConn(vault, scope string, c *connmodel.Connection) string {
	s.T().Helper()
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: vault})
	s.Require().NoError(err)
	key, err := secrets.Key(secrets.KindConn, scope, c.ConnID)
	s.Require().NoError(err)
	value, err := airflowenv.EncodeConnValue(*c)
	s.Require().NoError(err)
	s.Require().NoError(store.Set(key, value))
	return key
}

func pg(id, password string) *connmodel.Connection {
	return &connmodel.Connection{ConnID: id, ConnType: "postgres", ConnHost: "db.example", ConnLogin: "u", ConnPassword: password, ConnSchema: "analytics"}
}

// The reach rule, end to end: `astro otto` in a project writes the warehouses
// of exactly the connections that reach it — its own, the globals linked to it
// and the globals with no link row — and none linked elsewhere or to nothing.
// The YAML holds references; the secrets go only into Otto's environment, and
// no file under the skill's config dir holds one.
func (s *ConfigSuite) TestStartWritesTheWarehousesThatReachTheCheckout() {
	warehouses, vault := s.prepareLaunch()
	cwd := s.chdirManifestProject("reach-project")
	canon, err := localrt.CanonicalPath(cwd)
	s.Require().NoError(err)
	elsewhere := filepath.Join(s.T().TempDir(), "elsewhere")

	s.putConn(vault, canon, pg("own", "own-pw"))
	s.putConn(vault, secrets.GlobalScope, pg("everywhere", "everywhere-pw"))
	linked := s.putConn(vault, secrets.GlobalScope, pg("linked_here", "linked-pw"))
	other := s.putConn(vault, secrets.GlobalScope, pg("linked_elsewhere", "elsewhere-pw"))
	nowhere := s.putConn(vault, secrets.GlobalScope, pg("unlinked", "unlinked-pw"))
	// A global of the project connection's name: the project's wins.
	s.putConn(vault, secrets.GlobalScope, pg("own", "shadowed-pw"))
	s.Require().NoError(secrets.UpdateLinks(vault, func(rows map[string]secrets.Reach) error {
		rows[linked] = secrets.Reach{Projects: []string{canon}}
		rows[other] = secrets.Reach{Projects: []string{elsewhere}}
		rows[nowhere] = secrets.Reach{Projects: []string{}}
		return nil
	}))

	l := s.start()

	raw, err := os.ReadFile(filepath.Join(warehouses, "warehouse.yml"))
	s.Require().NoError(err)
	doc := map[string]any{}
	s.Require().NoError(yaml.Unmarshal(raw, &doc))
	var names []string
	for k := range doc {
		names = append(names, k)
	}
	s.ElementsMatch([]string{"airflow_own", "airflow_everywhere", "airflow_linked_here"}, names)
	info, err := os.Stat(filepath.Join(warehouses, "warehouse.yml"))
	s.Require().NoError(err)
	if runtime.GOOS != windowsGOOS {
		s.Equal(os.FileMode(0o600), info.Mode().Perm())
	}

	for key, want := range map[string]string{
		"AIRFLOW_OWN_PASSWORD":         "own-pw",
		"AIRFLOW_EVERYWHERE_PASSWORD":  "everywhere-pw",
		"AIRFLOW_LINKED_HERE_PASSWORD": "linked-pw",
	} {
		got, ok := l.get(key)
		s.True(ok && got == want, "Otto's environment lacks the value of %s", key)
	}
	for _, key := range []string{"AIRFLOW_LINKED_ELSEWHERE_PASSWORD", "AIRFLOW_UNLINKED_PASSWORD"} {
		_, ok := l.get(key)
		s.False(ok, "%s reached Otto's environment, but its connection does not reach this checkout", key)
	}

	_, err = os.Stat(filepath.Join(warehouses, ".env"))
	s.True(os.IsNotExist(err), "the launch wrote a .env")
	entries, err := os.ReadDir(warehouses)
	s.Require().NoError(err)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(warehouses, e.Name()))
		s.Require().NoError(err)
		s.False(strings.Contains(string(b), "-pw"), "%s holds a connection secret", e.Name())
	}
}

// A user's own entries in the shared files survive the launch, a managed
// entry that no longer reaches the checkout is dropped, and the plaintext
// secrets an earlier launch left in .env are stripped.
func (s *ConfigSuite) TestStartPreservesUserWarehouseEntries() {
	warehouses, vault := s.prepareLaunch()
	s.chdirManifestProject("preserve-project")
	s.Require().NoError(os.MkdirAll(warehouses, 0o750))
	s.Require().NoError(os.WriteFile(filepath.Join(warehouses, "warehouse.yml"),
		[]byte("mine:\n  type: duckdb\nairflow_gone:\n  type: postgres\n"), 0o600))
	s.Require().NoError(os.WriteFile(filepath.Join(warehouses, ".env"),
		[]byte("MY_TOKEN=keep\n\n# --- Astro: Airflow connection secrets (regenerated on change; do not edit) ---\n"+
			"AIRFLOW_GONE_PASSWORD=stale\nAIRFLOW_FRESH_PASSWORD=old-fresh\n"), 0o600))
	s.putConn(vault, secrets.GlobalScope, pg("fresh", "fresh-pw"))

	l := s.start()

	raw, err := os.ReadFile(filepath.Join(warehouses, "warehouse.yml"))
	s.Require().NoError(err)
	s.Contains(string(raw), "mine:")
	s.Contains(string(raw), "airflow_fresh:")
	s.NotContains(string(raw), "airflow_gone")
	env, err := os.ReadFile(filepath.Join(warehouses, ".env"))
	s.Require().NoError(err)
	s.Equal("MY_TOKEN=keep\n", string(env))
	fresh, _ := l.get("AIRFLOW_FRESH_PASSWORD")
	s.True(fresh == "fresh-pw", "AIRFLOW_FRESH_PASSWORD in Otto's environment is not the vault's value")
}

// A key the launcher's own environment already sets keeps its value: the
// skill's .env load never overrode the process environment, and a launch does
// not either.
func (s *ConfigSuite) TestStartKeepsAnInheritedWarehouseKey() {
	_, vault := s.prepareLaunch()
	s.chdirManifestProject("inherit-project")
	s.putConn(vault, secrets.GlobalScope, pg("mine", "vault-pw"))
	s.T().Setenv("AIRFLOW_MINE_PASSWORD", "shell-pw")

	l := s.start()

	n := 0
	for _, e := range l.env {
		if strings.HasPrefix(e, "AIRFLOW_MINE_PASSWORD=") {
			n++
		}
	}
	s.Equal(1, n, "AIRFLOW_MINE_PASSWORD set more than once")
	got, _ := l.get("AIRFLOW_MINE_PASSWORD")
	s.True(got == "shell-pw", "the vault's value replaced the inherited AIRFLOW_MINE_PASSWORD")
}

// --version exits before Otto reads anything: no warehouse files, and so no
// keychain access for them.
func (s *ConfigSuite) TestStartVersionWritesNoWarehouses() {
	warehouses, vault := s.prepareLaunch()
	s.chdirManifestProject("version-project")
	s.putConn(vault, secrets.GlobalScope, pg("any", "any-pw"))

	s.start("--version")

	_, err := os.Stat(warehouses)
	s.True(os.IsNotExist(err), "warehouse config written for --version")
}

// Otto's `astro` is the CLI that launched it: a launcher directory holding
// only an `astro` that resolves to the real binary comes first on PATH, then
// ~/.astro/bin, then the inherited PATH. The CLI's own directory is not on it,
// so nothing installed beside the CLI (python, uv, airflow) is exposed.
func (s *ConfigSuite) TestStartPutsOnlyTheLauncherFirstOnPath() {
	s.prepareLaunch()
	s.chdirTempProject("path-project")
	realDir := s.T().TempDir()
	realExe := filepath.Join(realDir, "astro")
	s.Require().NoError(os.WriteFile(realExe, []byte("#!/bin/sh\n"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(realDir, "python"), []byte("#!/bin/sh\n"), 0o755))
	linkDir := s.T().TempDir()
	link := filepath.Join(linkDir, "astro")
	if err := os.Symlink(realExe, link); err != nil {
		if runtime.GOOS == windowsGOOS && errors.Is(err, errPrivilegeNotHeld) {
			s.T().Skip("creating a symlink needs a privilege this Windows account lacks")
		}
		s.Require().NoError(err)
	}
	origExe := executable
	executable = func() (string, error) { return link, nil }
	s.T().Cleanup(func() { executable = origExe })
	s.T().Setenv("PATH", "/usr/bin")
	// Left over from an earlier launch: it must not survive the refresh.
	s.Require().NoError(os.MkdirAll(LauncherBinDir(), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(LauncherBinDir(), "uv"), []byte("x"), 0o755))

	l := s.start()

	path, ok := l.get("PATH")
	s.Require().True(ok)
	sep := string(os.PathListSeparator)
	s.Equal(strings.Join([]string{LauncherBinDir(), BinDir(), "/usr/bin"}, sep), path)
	s.NotContains(path, realDir)
	s.NotContains(path, linkDir)

	entries, err := os.ReadDir(LauncherBinDir())
	s.Require().NoError(err)
	s.Require().Len(entries, 1, "the launcher directory exposes something besides astro")
	s.Equal(launcherName(), entries[0].Name())
	launcher := filepath.Join(LauncherBinDir(), launcherName())
	if runtime.GOOS == windowsGOOS {
		// A copy, since making a symlink there needs a privilege most users lack.
		got, err := os.ReadFile(launcher)
		s.Require().NoError(err)
		s.Equal("#!/bin/sh\n", string(got))
	} else {
		// One hop, straight to the real file, not to the link that launched it.
		target, err := os.Readlink(launcher)
		s.Require().NoError(err)
		want, err := filepath.EvalSymlinks(realExe)
		s.Require().NoError(err)
		s.Equal(want, target)
	}
	s.Equal(BinaryPath(), l.bin)
	// Otto is told which astro launched it, so it asks this CLI for tokens.
	cliPath, ok := l.get(CLIPathEnv)
	s.True(ok)
	s.Equal(launcher, cliPath)
}

// When the launcher entry cannot be made, Otto is still told which astro
// launched it, rather than keeping an ASTRO_CLI_PATH it inherited.
func (s *ConfigSuite) TestStartNamesTheLauncherWithoutALauncherEntry() {
	s.prepareLaunch()
	s.chdirTempProject("no-launcher-project")
	exe := filepath.Join(s.T().TempDir(), "astro")
	s.Require().NoError(os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755))
	origExe := executable
	executable = func() (string, error) { return exe, nil }
	s.T().Cleanup(func() { executable = origExe })
	s.T().Setenv(CLIPathEnv, "/somewhere/else/astro")
	// A file where the launcher directory's parent should be.
	s.Require().NoError(os.RemoveAll(filepath.Dir(LauncherBinDir())))
	s.Require().NoError(os.MkdirAll(filepath.Dir(filepath.Dir(LauncherBinDir())), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Dir(LauncherBinDir()), []byte("x"), 0o600))

	l := s.start()

	cliPath, ok := l.get(CLIPathEnv)
	s.True(ok)
	s.Equal(exe, cliPath)
}

// Windows spells the key "Path". Prepending must extend that entry, not add
// a second PATH beside it.
func (s *ConfigSuite) TestPrependPathMatchesWindowsCase() {
	sep := string(os.PathListSeparator)
	env := prependPath([]string{"A=1", `Path=C:\Windows`}, `C:\launcher`, true)
	s.Equal([]string{"A=1", `Path=C:\launcher` + sep + `C:\Windows`}, env)

	// Case-sensitive elsewhere: "Path" is some other variable there.
	env = prependPath([]string{"Path=x"}, "/l", false)
	s.Equal([]string{"Path=x", "PATH=/l"}, env)
}

// A project's Airflow gets its URL and no default account; Otto resolves
// that project's credentials itself.
func (s *ConfigSuite) TestStartProjectAirflowGetsNoAccount() {
	s.prepareLaunch()
	srv := s.startFakeAirflow()
	defer srv.Close()
	cwd := s.chdirManifestProject("project-launch")
	s.writeProjectRecord(cwd, serverPort(srv))
	s.T().Setenv("AIRFLOW_USERNAME", "") // restored after; unset for the run
	s.T().Setenv("AIRFLOW_PASSWORD", "")
	s.Require().NoError(os.Unsetenv("AIRFLOW_USERNAME"))
	s.Require().NoError(os.Unsetenv("AIRFLOW_PASSWORD"))

	l := s.start()

	url, _ := l.get("AIRFLOW_API_URL")
	s.Equal(fmt.Sprintf("http://localhost:%d", serverPort(srv)), url)
	_, hasUser := l.get("AIRFLOW_USERNAME")
	_, hasPass := l.get("AIRFLOW_PASSWORD")
	s.False(hasUser)
	s.False(hasPass)
}

// A v1 route keeps the default account.
func (s *ConfigSuite) TestStart1xAirflowKeepsTheAccount() {
	s.prepareLaunch()
	srv := s.startFakeAirflow()
	defer srv.Close()
	cwd := s.chdirTempProject("1x-launch")
	s.writeRoute(&pkgproxy.Route{Port: urlPort(s.T(), srv.URL), ProjectDir: cwd, PID: os.Getpid()})

	l := s.start()

	url, _ := l.get("AIRFLOW_API_URL")
	s.Equal("http://localhost:"+urlPort(s.T(), srv.URL), url)
	user, _ := l.get("AIRFLOW_USERNAME")
	pass, _ := l.get("AIRFLOW_PASSWORD")
	s.Equal("admin", user)
	s.Equal("admin", pass)
}

func detectedURL() string {
	url, _ := DetectAirflow()
	return url
}
