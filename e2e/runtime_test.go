//go:build e2e && !windows

package e2e

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Tier 2: a real Airflow process. Everything below starts one, which is what
// separates these from tier 1 — the same project, the same environment, but
// now something is listening.
//
// Constrained to unix because the stale-record case has to end a process group
// the way a crash would, and because tier 2 has no Windows runner to run on.
// The tier gate would skip these there anyway; the constraint is so the file
// compiles.
//
// Measured on a warm developer machine: start 11s, stop 2s. The cost of a case
// here is one of those cycles, which is why they are counted rather than
// added freely.

const (
	// httpProbeTimeout bounds asking whether the port serves. Generous,
	// because a slow answer is still an answer.
	httpProbeTimeout = 10 * time.Second
	// dialProbeTimeout bounds asking whether the port is refused, and is short
	// on purpose: it runs inside a polling loop whose declared limit would
	// otherwise be a fiction. See waitFor.
	dialProbeTimeout = 200 * time.Millisecond
)

// startedProject is a scaffolded project with its environment built and a real
// Airflow running, stopped again however the test ends.
//
// The cleanup is the load-bearing part. A case that fails midway must not
// leave an Airflow holding a port and a record on the machine that ran it, so
// the stop is registered before anything can fail.
func startedProject(t *testing.T) (*project, rtStatus) {
	t.Helper()
	return startedNamedProject(t, "project")
}

func startedNamedProject(t *testing.T, name string) (*project, rtStatus) {
	t.Helper()
	p := namedAirflowProject(t, name)
	return p, startAndStop(t, p)
}

// startAndStop starts p's Airflow and registers the stop that ends it, however
// the test ends.
//
// The cleanup is the load-bearing part: a case that fails midway must not
// leave an Airflow holding a port on the machine that ran it, so the stop is
// registered before the start can fail. Stopping an already-stopped project is
// not a failure worth reporting — the record is simply gone, and a non-zero
// exit is recorded rather than fatal — while a stop that TIMES OUT does fail
// the test, which is the case worth hearing about.
func startAndStop(t *testing.T, p *project) rtStatus {
	t.Helper()
	t.Cleanup(func() { p.runSlow("local", "stop") })
	p.runSlow("local", "start").requireSuccess()
	return p.status()
}

// rtStatus is the shape `astro local status --output json` publishes. Written
// out here rather than imported, for the reason devstub_test.go gives: this is
// the contract, and asking the implementation what it promises cannot catch a
// promise being broken.
type rtStatus struct {
	ProjectPath string `json:"projectPath"`
	Mode        string `json:"mode"`
	State       string `json:"state"`
	PID         int    `json:"pid"`
	Port        int    `json:"port"`
	Hostname    string `json:"hostname"`
	Airflow     string `json:"airflowMajor"`
}

func (p *project) status() rtStatus {
	p.t.Helper()
	var st rtStatus
	p.run("local", "status", "--output", "json").requireSuccess().requireJSON(&st)
	return st
}

// A started Airflow is running, reachable, and recorded; a stopped one is none
// of those.
//
// The whole of what `astro local start` promises, which no tier below this can
// check: the other tiers can prove the CLI says it started something, and only
// this one can prove something started.
func TestStartRunsAirflowAndStopEndsIt(t *testing.T) {
	tier(t, 2)

	p, st := startedProject(t)

	if st.State != "running" {
		t.Errorf("state = %q, want running", st.State)
	}
	if st.Mode != "standalone" {
		t.Errorf("mode = %q, want standalone (no --docker was passed)", st.Mode)
	}
	if st.PID <= 0 {
		t.Errorf("a running standalone Airflow should record its pid, got %d", st.PID)
	}
	if st.Port <= 0 {
		t.Fatalf("a running Airflow should record its port, got %d", st.Port)
	}
	// Written out in rtStatus to pin the contract, so it has to be read: a
	// field nothing asserts would let the CLI stop publishing it, or publish a
	// number where a string was, with every case here still green.
	if st.Airflow != "2" && st.Airflow != "3" {
		t.Errorf("airflowMajor = %q, want the generation as a string", st.Airflow)
	}

	// The port is the claim worth checking hardest: a record saying "running"
	// while nothing answers is the failure this tier exists to catch.
	if code := get(t, st.Port); code != http.StatusOK {
		t.Errorf("GET / on the recorded port = %d, want 200", code)
	}

	// And `list` agrees with `status`, since they read the same record by
	// different paths.
	row, ok := lineWith(p.run("local", "list").requireSuccess().Stdout, p.Dir)
	if !ok {
		t.Fatal("a running project should appear in `astro local list`")
	}
	for _, want := range []string{"running", "standalone", fmt.Sprint(st.Port)} {
		if !strings.Contains(row, want) {
			t.Errorf("the list row does not carry %q: %q", want, row)
		}
	}

	p.runSlow("local", "stop").requireSuccess()

	// Stopped, and the port is released. status still answers — it reports
	// the state of a project, and "nothing is running here" is a state — while
	// the record itself is gone, which is what `list` shows.
	stopped := p.status()
	if stopped.State != "stopped" {
		t.Errorf("state after stop = %q, want stopped", stopped.State)
	}
	if _, listed := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir); listed {
		t.Error("a stopped project should leave no record behind")
	}
	if !portClosed(t, st.Port) {
		t.Errorf("port %d still answers after stop", st.Port)
	}
}

// A record whose Airflow died is stale, and `list --clean` is what removes it.
//
// The crash is the point. Stopping properly removes the record on the way out,
// so the only way to get the state `--clean` exists for is to end the process
// without letting the CLI tidy up — which is what a kill -9, a reboot, or a
// laptop lid does.
func TestListCleanRemovesARecordWhoseAirflowDied(t *testing.T) {
	tier(t, 2)

	p, st := startedProject(t)

	// Checked before it is negated, and this is not a formality. The record
	// tags pid `omitempty`, so anything that leaves it unset decodes to 0 —
	// and kill(-0) is kill(0), which SIGKILLs every process in the caller's
	// own group: go test, make, the shell that started them. A pid of 1 is
	// worse. Refuse to sign anything but a real group.
	if st.PID <= 1 {
		t.Fatalf("refusing to signal process group %d; the status record gave pid %d", -st.PID, st.PID)
	}
	// The whole group, the way the supervisor's own stop does: killing the
	// supervisor alone leaves its Python children running and the port held.
	if err := syscall.Kill(-st.PID, syscall.SIGKILL); err != nil {
		t.Fatalf("ending the Airflow process group: %v", err)
	}
	waitFor(t, "the port to be released", func() bool { return portClosed(t, st.Port) })

	// The record outlived the process, which is what stale means.
	row, ok := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir)
	if !ok {
		t.Fatal("the record should still be listed after the process died")
	}
	if !strings.Contains(row, "stale") {
		t.Errorf("a record whose process is gone should read as stale: %q", row)
	}

	// Anchored before it is negated. The assertion below waits for the daemon
	// to go, and would pass just as contentedly if one had never been there:
	// EnsureDaemon is best-effort — a daemon that cannot spawn is a log line,
	// not a failed start — so "no proxy at all" is a reachable state, and in it
	// this case would report the leak fixed while nothing had been reaped.
	if !proxyRunning(p.home) {
		t.Fatal("no proxy daemon is running, so --clean has nothing to reap and this proves nothing")
	}

	cleaned := p.run("local", "list", "--clean").requireSuccess()
	if !strings.Contains(cleaned.Stdout, "Removed 1 stale record") {
		t.Errorf("--clean should report what it removed\n%s", cleaned.output())
	}
	if _, still := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir); still {
		t.Error("the stale record should be gone from disk")
	}

	// And the proxy that served the route goes too.
	//
	// `stop` has always reaped it; --clean did not, so the one command whose
	// job is to tidy up after a runtime that died badly left a daemon running
	// with nothing to serve, and nothing would ever collect it — the next
	// start adopts the daemon it finds rather than counting them. One leaked
	// per run of this suite, which has exactly this one --clean case.
	waitFor(t, "the proxy daemon to be reaped", func() bool { return !proxyRunning(p.home) })
}

// proxyRunning reports whether the proxy daemon under an ASTRO_HOME is alive,
// read from the record the daemon writes there.
//
// The record rather than a process scan: every project in a suite run spawns
// its own daemon from the same built binary, and each binds a port of its own,
// so neither the argv nor the port distinguishes them. The file is per-home and
// the home is per-project, which is the attribution this needs.
//
// A missing file and a dead pid are both "not running": the daemon removes its
// record on the way out, and one left behind by a kill names a pid that no
// longer answers. A pid the OS has since recycled would read as alive, which is
// the one way this can be wrong — the window is a whole pid space wide and the
// caller waits rather than asks once, so it is not worth a second signal.
//
// Takes the home rather than the project, and does not call t.Helper: it
// reports a bool and never fails a test, so there is no frame to hide.
func proxyRunning(home string) bool {
	raw, err := os.ReadFile(filepath.Join(home, ".astro", "proxy", "proxy.pid"))
	if err != nil {
		return false
	}
	// pkg/proxy writes "<pid> <version> <port>", pid first and space
	// separated, and has done since before the version and port were added —
	// fields are appended there, never reordered, precisely so a reader can
	// take the first and ignore the rest.
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 1 {
		return false
	}
	// Signal 0 asks without delivering: nil means some process owns that pid.
	return syscall.Kill(pid, 0) == nil
}

// Two projects running at once get a port each.
//
// Allocation is the kind of thing that looks right until two of them exist,
// and a second project silently reusing the first one's port is a failure that
// only a real start can show.
func TestTwoProjectsRunOnDifferentPorts(t *testing.T) {
	tier(t, 2)

	// One machine: the same ASTRO_HOME and the same cache, so both projects
	// register in one routes.json and the CLI's own allocation is what has to
	// keep them apart — not the operating system refusing a second bind.
	// Two projects with caches of their own are two machines as far as the CLI
	// can tell, and a regression in its bookkeeping would pass.
	//
	// Named apart because the hostname comes from the directory's base name,
	// and two directories both called "project" would collide for a reason
	// that has nothing to do with allocation.
	alpha := namedAirflowProject(t, "alpha")
	beta := alpha.sibling("beta")
	beta.run("init", "--name", "beta").requireSuccess()
	beta.sync()

	first := startAndStop(t, alpha)
	second := startAndStop(t, beta)

	if first.Port == second.Port {
		t.Fatalf("both projects took port %d", first.Port)
	}
	if first.Hostname == second.Hostname {
		t.Errorf("both projects took hostname %q", first.Hostname)
	}
	for _, st := range []rtStatus{first, second} {
		if code := get(t, st.Port); code != http.StatusOK {
			t.Errorf("GET / on port %d = %d, want 200", st.Port, code)
		}
	}
}

// Two projects whose directories share a base name.
//
// The hostname comes from that base name, so both ask for alpha.localhost —
// which is why the port case above goes out of its way to name its two
// projects differently. The second one used to lose the draw: AddRoute
// refused the duplicate, the refusal was a log line rather than anything the
// start acted on, and the project came up with no name of its own while
// alpha.localhost went on resolving to the first. Opening it showed somebody
// else's Airflow, which is worse than showing nothing.
//
// Both now get a name. The holder keeps the plain one and the newcomer takes
// alpha-<id6>.localhost, built from the path hash so it is the same name on
// every restart.
//
// Tier 2 because only a real start registers a route; the rule and both
// engines' wiring are checked in pkg/localrt, and what this adds is that the
// two halves meet.
func TestTwoProjectsOfTheSameNameGetDistinctHostnames(t *testing.T) {
	tier(t, 2)

	// One ASTRO_HOME and one cache, as above: two projects sharing a
	// routes.json is the only arrangement in which they can collide at all.
	alpha := namedAirflowProject(t, "alpha")
	twin := alpha.sibling("alpha")
	twin.run("init", "--name", "alpha").requireSuccess()
	twin.sync()

	if filepath.Base(alpha.Dir) != filepath.Base(twin.Dir) {
		t.Fatalf("the fixture is not the case: %q and %q do not share a base name",
			alpha.Dir, twin.Dir)
	}
	if alpha.Dir == twin.Dir {
		t.Fatalf("both projects are the same directory %q", alpha.Dir)
	}

	first := startAndStop(t, alpha)
	second := startAndStop(t, twin)

	if first.Hostname == second.Hostname {
		t.Fatalf("both projects took hostname %q, so one of them is unreachable by name", first.Hostname)
	}
	base := filepath.Base(alpha.Dir) + ".localhost"
	if first.Hostname != base {
		t.Errorf("the project that got there first should keep the plain name: got %q, want %q",
			first.Hostname, base)
	}
	if !strings.HasPrefix(second.Hostname, filepath.Base(twin.Dir)+"-") ||
		!strings.HasSuffix(second.Hostname, ".localhost") {
		t.Errorf("the second name should still say which project it is: got %q", second.Hostname)
	}

	// Both are still running and still answering, which is what makes the
	// distinct names worth anything.
	for _, st := range []rtStatus{first, second} {
		if code := get(t, st.Port); code != http.StatusOK {
			t.Errorf("GET / on port %d = %d, want 200", st.Port, code)
		}
	}

	// And both names answer through the proxy. Until here nothing asked the
	// proxy anything: the case compared recorded strings and probed the
	// direct ports, so two projects could hold two hostnames and neither be
	// routable.
	//
	// What this does NOT prove is that each name reaches its OWN Airflow — a
	// 200 from either backend passes. Proving that needs something in the
	// response that differs per project, and the obvious candidates (the
	// version endpoint, the login page) are identical. A per-project DAG and
	// an authenticated API call through the proxy would do it, which is more
	// machinery than the rest of this file has; the distinct-name half is
	// checked above, and the routing half is checked as far as "answers".
	// A subtest each, so a missing URL skips its own case rather than the
	// whole test: a bare t.Skip here would throw away the distinct-hostname
	// assertions above that already ran, turning a daemon regression into an
	// invisible skip instead of a loud failure.
	for _, c := range []struct {
		name string
		p    *project
		st   rtStatus
	}{{"first", alpha, first}, {"second", twin, second}} {
		t.Run("the "+c.name+" project answers on its own name", func(t *testing.T) {
			named := namedURL(t, c.p.forT(t), &c.st)
			if named == "" {
				t.Skip("the CLI offers the direct URL, so no proxy name to check")
			}
			if code := getURL(t, named); code != http.StatusOK {
				t.Errorf("GET %s = %d, want 200", named, code)
			}
		})
	}
}

// namedURL is the proxy URL a running project publishes, or "" when the CLI
// offered the direct one instead.
//
// Asked of the CLI rather than assembled here: the daemon picks its own port
// when the default is taken, so hostname plus a guessed port would test the
// guess. Compared against the direct form exactly rather than sniffed for
// substrings — cmd/local builds it as http://localhost:<port>, and a
// substring test would call a URL named whenever the record's hostname
// happened to be empty, since strings.Contains(s, "") is true.
//
// Empty covers two states this cannot tell apart: the daemon is down, or the
// record carries no hostname because no route was ever registered. The second
// is a regression and this reads it as an absent prerequisite — closing that
// needs the daemon's own port, which the CLI does not publish.
func namedURL(t *testing.T, p *project, st *rtStatus) string {
	t.Helper()
	var res struct {
		URL string `json:"url"`
	}
	p.run("local", "open", "--print", "--output", "json").requireSuccess().requireJSON(&res)
	if res.URL == fmt.Sprintf("http://localhost:%d", st.Port) {
		return ""
	}
	return res.URL
}

// The five things you ask a running Airflow, on one start.
//
// Sharing a fixture rather than a case each: this tier costs a start-and-stop
// cycle per case (~11s and ~2s measured), and these are read-only queries
// against the same running Airflow.
//
// Each subtest takes a project view bound to its own T. Without that, a
// *result carries the T it was made from, so require* inside a subtest calls
// Fatalf on the parent — a FailNow from the wrong goroutine, which aborts the
// run and silently skips every sibling after it. Grouping to isolate failures
// achieves the opposite if the binding is left alone.
func TestQueriesAgainstARunningAirflow(t *testing.T) {
	tier(t, 2)

	parent, st := startedProject(t)

	// `local af health` — the four-part report and its verdict. Every
	// section is read, per this file's own rule: a field nothing asserts is
	// a field the CLI can stop publishing.
	t.Run("af health reports on the running Airflow", func(t *testing.T) {
		p := parent.forT(t)
		var report struct {
			OverallStatus string `json:"overall_status"`
			Version       struct {
				Version string `json:"version"`
				Error   string `json:"error"`
			} `json:"version"`
			ImportErrors struct {
				Count int    `json:"count"`
				Error string `json:"error"`
			} `json:"import_errors"`
			DAGWarnings struct {
				Count int    `json:"count"`
				Error string `json:"error"`
			} `json:"dag_warnings"`
			DAGStats struct {
				Error string `json:"error"`
			} `json:"dag_stats"`
			Unread []string `json:"unread"`
		}
		p.runSlow("local", "af", "health", "--output", "json").
			requireSuccess().
			requireJSON(&report)

		if report.OverallStatus != "healthy" {
			t.Errorf("overall_status = %q, want healthy for an Airflow that just started", report.OverallStatus)
		}
		if len(report.Unread) != 0 {
			t.Errorf("health could not read %v; a section it cannot read is the failure this reports", report.Unread)
		}
		// The four sections, each reported as failed rather than ending the
		// command — so an error field is how a degraded report shows itself.
		if report.Version.Version == "" || report.Version.Error != "" {
			t.Errorf("version section: %+v", report.Version)
		}
		for name, errText := range map[string]string{
			"import_errors": report.ImportErrors.Error,
			"dag_warnings":  report.DAGWarnings.Error,
			"dag_stats":     report.DAGStats.Error,
		} {
			if errText != "" {
				t.Errorf("%s section reported an error: %s", name, errText)
			}
		}
	})

	// `local run` must use the PROJECT's interpreter, not the machine's,
	// which is the whole claim. Compared against this project's own path:
	// ".venv" alone would accept an ambient venv leaked through the
	// environment, which is the mistake worth catching.
	t.Run("run uses this project's interpreter", func(t *testing.T) {
		p := parent.forT(t)
		out := strings.TrimSpace(p.runSlow("local", "run", "python", "-c",
			"import sys; print(sys.executable)").requireSuccess().Stdout)

		// The venv DIRECTORY on both sides, never the interpreter: a venv's
		// bin/python is a symlink to the base interpreter, so resolving the
		// binary lands on Homebrew's python and reports the project's own
		// venv as the machine's. Resolving the directory is still needed for
		// /var against /private/var on macOS.
		want, err := filepath.EvalSymlinks(filepath.Join(p.Dir, ".venv"))
		if err != nil {
			t.Fatalf("resolving the project venv: %v", err)
		}
		got, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(out)))
		if err != nil {
			t.Fatalf("resolving the venv behind %q: %v", out, err)
		}
		if got != want {
			t.Errorf("`local run` used %q, whose venv is %q, want %q", out, got, want)
		}
	})

	// And reaches the Airflow that is running, which `airflow version` does
	// not: that prints the installed package's version and talks to nothing.
	// `dags list` reads the metadata database this start migrated.
	t.Run("run reaches the running Airflow's database", func(t *testing.T) {
		p := parent.forT(t)
		out := p.runSlow("local", "run", "airflow", "dags", "list").requireSuccess().Stdout
		if strings.TrimSpace(out) == "" {
			t.Error("`local run airflow dags list` printed nothing; it should at least render a header")
		}
	})

	// `--tail N` is the feature, not the logs: unbounded output from a busy
	// Airflow is unreadable. Exactly min(N, total) — an upper bound alone
	// passes for a --tail that returns nothing.
	t.Run("logs --tail returns exactly the last N", func(t *testing.T) {
		p := parent.forT(t)
		total := countNonEmptyLines(p.runSlow("local", "logs").requireSuccess().Stdout)
		if total == 0 {
			t.Skip("nothing logged yet; --tail has nothing to bound")
		}
		const n = 5
		got := countNonEmptyLines(p.runSlow("local", "logs", "--tail", fmt.Sprint(n)).requireSuccess().Stdout)

		if want := min(n, total); got != want {
			t.Errorf("--tail %d returned %d lines of %d, want %d", n, got, total, want)
		}
	})

	// The named URL is why the proxy exists; the direct port is covered by
	// TestStartRunsAirflowAndStopEndsIt.
	t.Run("the named URL answers", func(t *testing.T) {
		p := parent.forT(t)
		named := namedURL(t, p, &st)
		if named == "" {
			t.Skip("the CLI offers the direct URL, so no proxy name to check")
		}
		if code := getURL(t, named); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", named, code)
		}
	})
}

// countNonEmptyLines counts what --tail bounds: a blank trailing line is
// formatting, not output.
func countNonEmptyLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// `astro local reset` wipes the derived state and leaves the project alone.
//
// The distinction is the whole command: the database and logs are rebuildable,
// the DAGs are not, and a reset that took the second with the first would be
// the worst bug in this tree.
func TestResetWipesTheDerivedStateAndKeepsTheProject(t *testing.T) {
	tier(t, 2)

	p, _ := startedProject(t)
	dag := filepath.Join(p.Dir, "dags", "exampledag.py")
	before := read(t, dag)

	// The environment a run derives, which Reset's doc lists among what it
	// removes. Checked before, so "it is gone afterwards" means something:
	// asserting only the absence would pass on a project that never had one.
	venv := filepath.Join(p.Dir, ".venv")
	if _, err := os.Stat(venv); err != nil {
		t.Fatalf("expected a built environment before the reset: %v", err)
	}

	p.runSlow("local", "reset", "--yes").requireSuccess()

	// Half the command is what it removes, and a reset that stopped Airflow
	// and cleared the record while leaving the derived state on disk would
	// pass every other assertion here.
	if _, err := os.Stat(venv); !os.IsNotExist(err) {
		t.Errorf("the derived environment survived the reset (stat: %v)", err)
	}

	// The project's own files are untouched.
	if got := read(t, dag); got != before {
		t.Error("reset rewrote a DAG; it may only remove derived state")
	}
	// Stat rather than read: read Fatalfs on a missing file, so a reset that
	// deleted the manifest — the regression this line is for — would die with
	// a bare ENOENT instead of saying what happened.
	if _, err := os.Stat(filepath.Join(p.Dir, "pyproject.toml")); err != nil {
		t.Errorf("reset removed the manifest: %v", err)
	}

	// And nothing is running or recorded afterwards.
	if after := p.status(); after.State != "stopped" {
		t.Errorf("state after reset = %q, want stopped", after.State)
	}
	if _, listed := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir); listed {
		t.Error("reset should leave no runtime record")
	}
}

// get is one HTTP GET against a local port, returning the status code or 0.
func get(t *testing.T, port int) int {
	t.Helper()
	return getURL(t, fmt.Sprintf("http://localhost:%d/", port))
}

// getURL is get for a whole URL, which the named <project>.localhost address
// needs: the proxy decides where to send a request by its Host header, so a
// port alone cannot ask the question.
//
// The connection goes to 127.0.0.1 with Host set, rather than letting the
// resolver look up <project>.localhost. Go's resolver has no built-in rule
// for .localhost — it works on a developer's Mac and wherever
// systemd-resolved synthesizes it, and returns a connection error elsewhere.
// The nightly runs on ubuntu-latest, where a missing rule would file an issue
// reading as a proxy regression. airflow/proxy's own probeProxySignature asks
// the same way and for the same reason.
func getURL(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Errorf("parsing %q: %v", rawURL, err)
		return 0
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://127.0.0.1:"+port+u.EscapedPath(), http.NoBody)
	if err != nil {
		t.Errorf("building a request for %q: %v", rawURL, err)
		return 0
	}
	// What the proxy routes on.
	req.Host = u.Host

	resp, err := (&http.Client{Timeout: httpProbeTimeout}).Do(req)
	if err != nil {
		t.Logf("GET %s (as 127.0.0.1 with Host %s): %v", rawURL, u.Host, err)
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// portClosed reports whether the port refuses connections.
//
// Refusal specifically, not "the probe did not succeed". An Airflow partway
// through shutting down still accepts the connection and then says nothing,
// and reading that as closed would let the assertion this tier exists for —
// that stop really stops — pass against the regression it is watching for.
func portClosed(t *testing.T, port int) bool {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), dialProbeTimeout)
	if err == nil {
		conn.Close()
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	// Anything else (a timeout, a reset midway) says the port is not cleanly
	// free, which is the answer that keeps a caller waiting rather than the
	// one that lets it conclude.
	return false
}

// waitForLimit bounds every waitFor. One value rather than an argument,
// because every caller wants the same thing from it: long enough that a
// machine under load is not called a failure, short enough that a real hang
// is reported rather than waited out. Nothing here is waiting on a human.
const waitForLimit = 30 * time.Second

// waitFor polls until cond holds, and fails the test naming what it waited
// for. Used for what is genuinely asynchronous after a teardown: a signaled
// process group going, and the port it held coming free.
//
// The bound is only honest if a probe is short next to it, which is why the
// dial timeout below is a fraction of the poll interval rather than the ten
// seconds an HTTP client would take — otherwise "waited 30s" could mean forty,
// over three samples.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitForLimit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("waited %s for %s", waitForLimit, what)
}
