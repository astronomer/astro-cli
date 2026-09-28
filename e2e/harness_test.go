//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// testVersion is stamped into the binary under test.
//
// A fixed value makes `astro version` assertable, but the shape is the load
// bearing part: the CLI's pre-run hook compares its version against the release
// list on every command, over the network, unless the version reads as a
// snapshot. Stamp this "1.45.0" and the whole suite grows an HTTP request per
// command, each with a three-second timeout, against a service that has nothing
// to do with what is being tested.
const testVersion = "SNAPSHOT-e2e"

// commandTimeout bounds a single CLI invocation. A hung command should fail its
// own test with its output, rather than run down the whole suite's -timeout and
// take every other case's diagnosis with it.
const commandTimeout = 90 * time.Second

// slowCommandTimeout bounds a command that starts or stops a real Airflow,
// which the ordinary bound is far too tight for. See runSlow.
//
// Strictly greater than what the CLI itself may spend, which is the whole
// trick: localstandalone's defaultHealthTimeout waits five minutes for
// Airflow to come up, and provisioning happens before that wait even starts.
// A harness bound of five minutes would therefore kill `astro local start` at
// the moment the CLI was about to report a clean, diagnosable failure — and
// kill it badly, since the supervisor is in its own process group and would
// survive to hold the port. Ten leaves room for the CLI to lose first and say
// why.
const slowCommandTimeout = 10 * time.Minute

// astroBin is the binary under test, built once for the whole run.
var astroBin string

// uvCache is where uv keeps its downloads, captured before any test isolates
// HOME.
//
// It is deliberately NOT isolated, and it is the one thing here that is not.
// uv's cache lives under XDG_CACHE_HOME (or ~/Library/Caches), both of which a
// test redirects, so leaving it alone means a case that builds a Python
// environment refills it: measured at 2.3s and 221 MB per case against 254ms
// with it shared. It is a content-addressed download cache that uv owns and
// evicts, so sharing it cannot change what a test observes — only how long it
// waits.
//
// This reaches the uv the SUITE runs (see sync). The uv the CLI runs ignores
// the variable entirely and needs shareUVCache as well; the two together are
// what make the whole of tier 1 share one cache.
//
// $UV_CACHE_DIR wins when set, which is how CI points it at a path it restores
// between runs.
var uvCache string

func init() {
	if dir := os.Getenv("UV_CACHE_DIR"); dir != "" {
		uvCache = dir
		return
	}
	// uv's default on Linux and macOS, computed from the real environment
	// rather than the redirected one a test runs under. On Windows this is
	// %LOCALAPPDATA%\uv, where uv itself uses %LOCALAPPDATA%\uv\cache — a
	// cache of our own beside uv's rather than uv's, which costs a Windows
	// developer one cold fill and nothing else. Tier 1 does not run on Windows
	// in CI, which is why it is not worth shelling out to uv to ask.
	if dir, err := os.UserCacheDir(); err == nil {
		uvCache = filepath.Join(dir, "uv")
	}
}

func TestMain(m *testing.M) {
	code, err := runSuite(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

// runSuite exists so the build's temp directory can be cleaned up with a defer,
// which os.Exit in TestMain would otherwise skip.
func runSuite(m *testing.M) (int, error) {
	dir, err := os.MkdirTemp("", "astro-e2e-bin")
	if err != nil {
		return 0, fmt.Errorf("making a directory for the binary: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := buildAstro(dir); err != nil {
		return 0, err
	}

	// Taken after the build, so the census is about what the CASES do. The
	// census's own doc says what it covers and what it cannot.
	//
	// A ceiling that does not parse censuses nothing: the first tier() call
	// fails the run and says why, and guessing a tier here would either skip a
	// check that was wanted or pay for one that cannot find anything.
	ceiling, ok := tierCeiling()
	if !ok {
		ceiling = 0
	}
	before := takeCensus(context.Background(), ceiling)

	code := m.Run()

	// Reported even when the run already failed: a case that fails partway is
	// the one most likely to have skipped its own teardown, and that is exactly
	// when knowing what is still on the machine is worth having.
	if reportLeaks(before, ceiling) && code == 0 {
		code = 1
	}
	return code, nil
}

// dockerListTimeout bounds one docker listing. Generous, because it is a
// backstop against an engine that has stopped answering rather than a budget:
// a listing that hangs turns a passing suite into a -timeout panic, which
// dumps goroutines instead of naming the case and skips its cleanup.
const dockerListTimeout = 30 * time.Second

// dockerLines runs a docker listing and returns its non-empty lines.
//
// Shared by the cases that ask docker what exists and by the leak census, so it
// lives here rather than in either: a bound named for one of them would end up
// silently deciding the other.
//
// exec.CommandContext rather than a timeout raced against cmd.Output: the suite
// runs under -race, and killing the process from a second goroutine reads
// cmd.Process while Start is writing it.
func dockerLines(ctx context.Context, args ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, dockerListTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// buildAstro builds the CLI from the repository this module sits in.
func buildAstro(dir string) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	name := "astro"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	astroBin = filepath.Join(dir, name)

	ldflags := "-X github.com/astronomer/astro-cli/version.CurrVersion=" + testVersion
	// Package mode ("." rather than main.go), which is how the binary is
	// released. File mode also disables the toolchain's VCS stamping, so a
	// build made that way answers differently about its own version.
	cmd := exec.Command("go", "build", "-o", astroBin, "-ldflags", ldflags, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("building the CLI: %w\n%s", err, out)
	}
	return nil
}

// repoRoot is this module's parent directory, checked rather than assumed: a
// wrong answer here builds nothing and reports it as a test failure.
func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolving the working directory: %w", err)
	}
	root := filepath.Dir(wd)
	if _, err := os.Stat(filepath.Join(root, "main.go")); err != nil {
		return "", fmt.Errorf("expected the root module at %s, found no main.go: %w", root, err)
	}
	return root, nil
}

// tierCeiling reads ASTRO_E2E_MAX_TIER. ok is false when it is set to something
// that is not a number.
//
// Split out of maxTier so that TestMain can read the ceiling too: it has no
// *testing.T, and the leak census uses the ceiling to decide whether asking
// docker anything can tell it something.
func tierCeiling() (n int, ok bool) {
	raw := os.Getenv("ASTRO_E2E_MAX_TIER")
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

// maxTier is the highest tier this run executes. See the package doc.
func maxTier(t *testing.T) int {
	t.Helper()
	n, ok := tierCeiling()
	if !ok {
		// Not a skip: a typo here would silently run the hermetic tier and
		// report green for a suite the caller believed covered Docker.
		t.Fatalf("ASTRO_E2E_MAX_TIER=%q is not a number", os.Getenv("ASTRO_E2E_MAX_TIER"))
	}
	return n
}

// tier declares what a test costs to run, and skips it when this run is not
// paying that much. Call it first in the test.
func tier(t *testing.T, n int) {
	t.Helper()
	if got := maxTier(t); n > got {
		t.Skipf("tier %d: this run stops at %d (raise it with ASTRO_E2E_MAX_TIER)", n, got)
	}
	declaredTiers.Store(t.Name(), n)
}

// declaredTiers is the tier each top-level test declared, for env to keep a
// tier 0 case off the network: see astroIndex.
var declaredTiers sync.Map

// declaredTier is the tier t declared, or the nearest test above it did.
func declaredTier(t *testing.T) int {
	for name := t.Name(); ; {
		if n, ok := declaredTiers.Load(name); ok {
			return n.(int)
		}
		i := strings.LastIndex(name, "/")
		if i < 0 {
			return 0
		}
		name = name[:i]
	}
}

// unreachableIndex stands in for Astronomer's package index in a tier 0 case.
// `astro init` looks the Airflow build up there, and the hermetic tier must
// neither reach the network nor write a manifest that depends on what it
// found. Offline, init writes no build and says nothing: the start makes the
// same lookup, and reports it.
const unreachableIndex = "http://127.0.0.1:1/v2/"

// project is one isolated place to run the CLI: a working directory, plus the
// three levers that keep the run out of the developer's real state.
type project struct {
	t *testing.T
	// Dir is the working directory commands run in.
	Dir string
	// home backs both ASTRO_HOME and HOME, as they coincide in the real thing.
	home  string
	cache string
	// catalogURL is the runtime catalog commands read, through
	// ASTRO_RUNTIME_VERSIONS_URL. Empty means unreachableCatalog, so no case
	// reaches Astronomer's catalog unless it asks to: `astro init` falls back
	// to its built-in series, and a case that needs a real catalog serves one
	// or names the live one.
	catalogURL string
}

// unreachableCatalog refuses connections at once, so a lookup fails fast rather
// than waiting out a timeout.
const unreachableCatalog = "http://127.0.0.1:1/astronomer-runtime"

// liveCatalog is Astronomer's real runtime catalog, for the tier 3 case that
// needs an Airflow 2 image lookup to name a real runtime.
const liveCatalog = "https://updates.astronomer.io/astronomer-runtime"

// forT returns a view of p whose failures land on t.
//
// A *result binds the T it was created from, so require* inside a subtest
// calls Fatalf on the PARENT — which is a FailNow from the wrong goroutine:
// Go reports "subtest may have called FailNow on a parent test", the subtest
// aborts, and every sibling after it is skipped silently. Grouping cases as
// subtests to isolate their failures does the opposite without this.
func (p *project) forT(t *testing.T) *project {
	sub := *p
	sub.t = t
	return &sub
}

// catalog is the runtime catalog address this project's commands read.
func (p *project) catalog() string {
	if p.catalogURL != "" {
		return p.catalogURL
	}
	return unreachableCatalog
}

// astroIndex is the package index this project's commands read: the real one
// from tier 1 up, where uv installs Airflow anyway, and none below.
func (p *project) astroIndex() string {
	if declaredTier(p.t) >= 1 {
		return "https://pip.astronomer.io/v2/"
	}
	return unreachableIndex
}

// newProject returns an isolated project directory. Everything it creates is
// under the test's own temp directory, so it goes away with the test.
func newProject(t *testing.T) *project {
	t.Helper()
	return newNamedProject(t, "project")
}

// newNamedProject is newProject with the working directory's name chosen.
//
// The name is not cosmetic: the proxy derives a project's hostname from its
// directory's base name, so two projects built by newProject are two
// directories both called "project" and both answering to project.localhost.
// A case about two projects at once has to name them apart to be about
// anything else.
func newNamedProject(t *testing.T, name string) *project {
	t.Helper()
	base := t.TempDir()
	p := &project{
		t:     t,
		Dir:   mkdir(t, base, name),
		home:  mkdir(t, base, "home"),
		cache: mkdir(t, base, "cache"),
	}
	p.shareUVCache()
	return p
}

// sibling is a second project on the same machine as p: its own directory, but
// the same ASTRO_HOME and the same cache, so the two share a record store and
// one routes.json.
//
// Which is the whole point for anything about allocation. Two projects built
// by newNamedProject are two machines as far as the CLI can tell — separate
// caches, separate route tables — so a port or hostname collision between them
// could only be caught by the operating system, not by the CLI's own
// bookkeeping, and a regression in that bookkeeping would pass.
func (p *project) sibling(name string) *project {
	p.t.Helper()
	s := &project{
		t:     p.t,
		Dir:   mkdir(p.t, p.t.TempDir(), name),
		home:  p.home,
		cache: p.cache,
	}
	s.shareUVCache()
	return s
}

// shareUVCache points the CLI's OWN uv cache at the shared one, which setting
// UV_CACHE_DIR does not do.
//
// pkg/uv drops any inherited UV_CACHE_DIR and pins the variable to
// <astro cache>/uv (see childEnv there, and newUVProvisioner in
// cmd/local/preflight.go) — deliberately, so the CLI's cache is somewhere it
// chose. That path is under the XDG_CACHE_HOME this project redirects, so a
// case that lets the CLI provision an interpreter downloads Airflow into a
// directory that goes away with the test: measured at ~10s per case, every
// run, and the CI cache cannot help it.
//
// So the astro cache stays isolated and only uv's subdirectory of it is
// shared. Same bargain as uvCache and for the same reason: uv owns a
// content-addressed store and evicts it itself, so sharing changes how long a
// case waits and nothing it observes.
//
// Best-effort. A symlink needs a privilege Windows does not grant by default,
// and a cold cache makes a case slow rather than wrong.
func (p *project) shareUVCache() {
	if uvCache == "" {
		return
	}
	if err := os.MkdirAll(uvCache, 0o755); err != nil {
		return
	}
	astroCache := filepath.Join(p.cache, "astro")
	if err := os.MkdirAll(astroCache, 0o755); err != nil {
		return
	}
	_ = os.Symlink(uvCache, filepath.Join(astroCache, "uv"))
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("making %s: %v", dir, err)
	}
	return dir
}

// leaky names the environment variables that would let the developer's own
// configuration reach a test. Dropped by prefix rather than by name, so a new
// ASTRO_* or AIRFLOW__* setting does not quietly become a dependency of the
// suite the first time someone exports it.
//
// UV_ and VIRTUAL_ENV are on the list because tier 1 builds a real Python
// environment: an exported UV_PYTHON, UV_INDEX_URL, UV_NO_CACHE or
// UV_PROJECT_ENVIRONMENT would steer the .venv six cases then assert against,
// and an activated VIRTUAL_ENV would capture the install outright — which is
// the same leak pkg/uv drops the variable for. The two UV_ settings this
// suite does want are put back in env below: UV_CACHE_DIR, and
// UV_EXCLUDE_NEWER at the suite's own date rather than the developer's.
var leaky = []string{
	"ASTRO_", "AIRFLOW_", "XDG_", "UV_",
	"HOME", "USERPROFILE", "NO_COLOR", "VIRTUAL_ENV",
}

// env is the whole environment a command runs with: the ambient one, minus
// anything that could steer the CLI, plus the isolation levers and extra.
//
// extra is applied last, so a case can set one of the variables the list below
// strips — AIRFLOW__CORE__DAGBAG_IMPORT_TIMEOUT, say, which decides a
// threshold a finding is judged against.
func (p *project) env(extra map[string]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || isLeaky(key) {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"ASTRO_HOME="+p.home,
		"XDG_CACHE_HOME="+p.cache,
		// Both spellings: os.UserHomeDir reads HOME on unix and USERPROFILE on
		// Windows, and it is what the vault resolves through.
		"HOME="+p.home,
		"USERPROFILE="+p.home,
		// Assertions are on text and JSON, not on escape sequences.
		"NO_COLOR=1",
		// What HOME redirection broke, put back deliberately — and the one
		// documented exception to the isolation contract above.
		//
		// `docker compose` is a CLI plugin, and docker looks for plugins under
		// the config directory, which defaults to $HOME/.docker. Redirecting
		// HOME therefore hides it, and docker answers a compose call with
		// "unknown command: docker compose" — which reads as a machine without
		// compose installed rather than as a test that moved it.
		//
		// This points at the developer's real config rather than a temp one,
		// and that is a choice with a cost. The same directory holds docker
		// contexts and registry credentials, and buildx writes its own state
		// into it during a build — so a tier-3 case reads and writes there.
		// A temp directory with cli-plugins symlinked in would restore
		// discovery without the rest, and would also break every developer
		// whose engine is reached through a context: Colima, Rancher Desktop
		// and rootless podman all keep the endpoint in that same config, so an
		// isolated one would point the suite at a default engine that is not
		// theirs.
		//
		// An ambient DOCKER_CONFIG wins, because a developer or a CI job that
		// set one has already said where docker's configuration lives, and
		// overriding it is how this reintroduces the error it exists to
		// remove.
		//
		// Set for every tier, not only the docker one, because the cost is a
		// variable nothing below tier 3 reads and the alternative is a lever
		// each case has to remember.
		"DOCKER_CONFIG="+dockerConfigDir(),
		// Not optional. The CLI's own guard against tracking its test runs
		// recognizes a Go test binary by its ".test" suffix, and this suite
		// drives a binary named "astro" — so without this, every case spawns a
		// sender and posts an event to production analytics. It also suppresses
		// the first-run notice, which is printed from the same code path and
		// writes to the config as a side effect.
		"ASTRO_TELEMETRY_DISABLED=1",
		// ASTRO_ is stripped above, so the catalog a command reads is set
		// here, deliberately: see catalogURL.
		"ASTRO_RUNTIME_VERSIONS_URL="+p.catalog(),
		"ASTRO_AIRFLOW_INDEX_URL="+p.astroIndex(),
	)
	// See uvCache: shared on purpose. Only when there is a directory to name —
	// an explicitly empty UV_CACHE_DIR is not the same as an unset one, and uv
	// may resolve it relative to the working directory, dropping a cache tree
	// inside the very project `astro package` then walks.
	if uvCache != "" {
		env = append(env, "UV_CACHE_DIR="+uvCache)
	}
	// See pinnedExcludeNewer: every uv this command reaches, the CLI's
	// included, resolves as of one date rather than against live PyPI.
	if cutoff := excludeNewer(); cutoff != "" {
		env = append(env, "UV_EXCLUDE_NEWER="+cutoff)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// dockerConfigDir is where docker's configuration lives for this run: what the
// environment already says, or the real user's directory resolved from the
// TEST process's own home rather than the redirected one the CLI gets.
//
// Empty when there is no home to read, which leaves DOCKER_CONFIG set to the
// empty string — docker then falls back to its own default, which is the
// behavior of a run that never set it.
func dockerConfigDir() string {
	if set := os.Getenv("DOCKER_CONFIG"); set != "" {
		return set
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker")
}

func isLeaky(key string) bool {
	upper := strings.ToUpper(key)
	for _, l := range leaky {
		if upper == l || (strings.HasSuffix(l, "_") && strings.HasPrefix(upper, l)) {
			return true
		}
	}
	return false
}

// result is what one CLI invocation produced.
type result struct {
	t        *testing.T
	Args     []string
	Stdout   string
	Stderr   string
	ExitCode int
}

// run invokes the CLI in this project and returns what happened. A command
// failing is data, not a test failure — most of the interesting assertions in
// this suite are about how the CLI refuses something.
func (p *project) run(args ...string) *result {
	p.t.Helper()
	return p.runWith(nil, args...)
}

// runSlow is run with the bound a command that drives a real Airflow needs.
//
// Tier 2 only. `astro local start` installs what the project is missing,
// migrates a database and waits for the API server to answer before it
// returns — 11s on a warm developer machine, and the case this bound exists
// for is a loaded CI runner doing the same work from cold.
func (p *project) runSlow(args ...string) *result {
	p.t.Helper()
	return p.runBounded(slowCommandTimeout, nil, args...)
}

// runWith invokes the CLI with extra environment on top of the isolated set.
func (p *project) runWith(extra map[string]string, args ...string) *result {
	p.t.Helper()
	return p.runBounded(commandTimeout, extra, args...)
}

func (p *project) runBounded(bound time.Duration, extra map[string]string, args ...string) *result {
	p.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()

	var stdout, stderr strings.Builder
	cmd := exec.CommandContext(ctx, astroBin, args...)
	cmd.Dir = p.Dir
	cmd.Env = p.env(extra)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	r := &result{t: p.t, Args: args, Stdout: stdout.String(), Stderr: stderr.String()}

	switch {
	case err == nil:
	case ctx.Err() != nil:
		p.t.Fatalf("`astro %s` did not finish within %s\n%s", strings.Join(args, " "), bound, r.output())
	default:
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			p.t.Fatalf("running `astro %s`: %v", strings.Join(args, " "), err)
		}
		r.ExitCode = exit.ExitCode()
	}
	return r
}

// output is both streams, labeled, for a failure message. Which stream a
// message lands on is itself often what is being asserted, so a failure should
// show both rather than leave the reader guessing.
func (r *result) output() string {
	return fmt.Sprintf("--- exit %d\n--- stdout\n%s\n--- stderr\n%s", r.ExitCode, r.Stdout, r.Stderr)
}

// requireFailure asserts the command failed, which for this CLI is the point of
// most of the cases here.
func (r *result) requireFailure() *result {
	r.t.Helper()
	if r.ExitCode == 0 {
		r.t.Fatalf("`astro %s` succeeded; it must fail\n%s", strings.Join(r.Args, " "), r.output())
	}
	return r
}

// requireSuccess asserts the command succeeded.
func (r *result) requireSuccess() *result {
	r.t.Helper()
	if r.ExitCode != 0 {
		r.t.Fatalf("`astro %s` failed\n%s", strings.Join(r.Args, " "), r.output())
	}
	return r
}

// requireStderr asserts a message reached stderr. The stream matters: guidance
// a script is meant to notice must not be on stdout, where `--output json`
// consumers parse.
func (r *result) requireStderr(want string) *result {
	r.t.Helper()
	if !strings.Contains(r.Stderr, want) {
		r.t.Errorf("stderr does not contain %q\n%s", want, r.output())
	}
	return r
}

// requireJSON decodes stdout into v. It asserts on the way that stdout is
// *only* the payload: a stray log line or banner is what breaks a consumer.
func (r *result) requireJSON(v any) {
	r.t.Helper()
	if err := json.Unmarshal([]byte(r.Stdout), v); err != nil {
		r.t.Fatalf("stdout is not the JSON payload it claims to be: %v\n%s", err, r.output())
	}
}

// requireLastJSON decodes the LAST line of stdout into v, for the commands that
// stream.
//
// A streaming surface emits NDJSON — one object per line, progress then result
// — so requireJSON's "stdout is only the payload" is the wrong assertion there
// and fails on the first build log. Each line still has to parse, because a
// half-written object in the middle of a stream is a consumer's problem too;
// only the last one is decoded into v.
func (r *result) requireLastJSON(v any) {
	r.t.Helper()
	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	var last string
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var probe any
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			r.t.Fatalf("stdout line %d is not JSON, so this is not a clean NDJSON stream: %v\n%s",
				i+1, err, r.output())
		}
		last = line
	}
	if last == "" {
		r.t.Fatalf("stdout carried no JSON at all\n%s", r.output())
	}
	if err := json.Unmarshal([]byte(last), v); err != nil {
		r.t.Fatalf("the last stdout line does not decode into the expected shape: %v\n%s", err, r.output())
	}
}
