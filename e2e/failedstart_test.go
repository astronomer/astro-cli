//go:build e2e && !windows

package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A start that cannot build the project's environment leaves nothing behind.
//
// The half of `astro local start` that no passing case can check. A start does
// several things that outlive the command — claims a port, reserves a
// hostname in routes.json, writes a record `list` and `status` read — and the
// failure worth guarding is the one where it does some of them and then gives
// up, leaving a project `list` calls running with nothing behind it. A port or
// hostname held by a dead record is worse than a failed start: the next start
// picks different ones, and the stale claim is only cleared by `--clean`.
//
// # Why this is tier 1 and not tier 2
//
// Airflow never runs here. The start dies in the provisioning phase, before
// anything is launched and long before the health wait — so this costs a uv
// resolution (~2s) rather than a real Airflow boot, and runs on every PR
// alongside the rest of tier 1. The expensive failure to induce is the health
// timeout, which is five minutes with no way to shorten it from outside the
// binary; this reaches the same cleanup path for the price of a resolve.
//
// # Network
//
// Deliberately network-dependent: proving a package does not exist means
// asking the index. A malformed requirement fails offline and a second
// faster, and would do as a test of the cleanup — but not of the message.
// uv reports that one through its build backend, so the CLI can name the
// phase and the backend that refused and no more; which dependency was wrong
// is in the streamed log rather than the error. A name the registry has never
// heard of is refused by uv itself, which is what lets this assert that the
// error names the package.
func TestAFailedStartLeavesNothingBehind(t *testing.T) {
	tier(t, 1)
	needsUV(t)

	p := newProject(t)
	p.run("init", "--name", "badstart").requireSuccess()
	unresolvable(t, p)

	r := p.run("local", "start").requireFailure()

	// Read before anything else runs. The route store prunes entries whose pid
	// is gone every time a command opens it, so `list` or `status` first would
	// clear a leaked route and leave this asserting the pruner's work rather
	// than the start's. Measured, not assumed: with a route deliberately
	// claimed before provisioning, this case passed until the read moved here.
	//
	// Asserted as "no routes at all" rather than "no route for this project":
	// newProject gives each case its own ASTRO_HOME holding exactly one
	// project, so any route here is this project's. Matching on the path
	// looked stricter and was weaker — the engine records the resolved path,
	// and a t.TempDir() under /var resolves to /private/var on macOS, so the
	// comparison was against a spelling that never appears in the file.
	if left := routes(t, p); len(left) != 0 {
		t.Errorf("a failed start left %d route(s) claimed: %+v", len(left), left)
	}

	// The message names the phase and carries uv's own explanation. A start
	// that fails silently, or that reports only "exit 1", sends its author to
	// read a log to find out that a dependency does not exist.
	for _, want := range []string{"preparing the project environment", missingPackage} {
		if !strings.Contains(r.Stderr, want) {
			t.Errorf("stderr does not carry %q\n%s", want, r.output())
		}
	}

	// Nothing recorded. --all is the wider question: it lists stopped records
	// too, so it catches a record written and then abandoned, which a plain
	// list would hide behind its running-only filter.
	if _, listed := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir); listed {
		t.Error("a failed start left a record in `astro local list --all`")
	}

	// And status agrees, by the other path: it reads the record store
	// directly rather than through the list's rendering.
	st := p.status()
	if st.State != "stopped" {
		t.Errorf("state after a failed start = %q, want stopped", st.State)
	}
	if st.PID != 0 {
		t.Errorf("a failed start recorded pid %d", st.PID)
	}
	if st.Port != 0 {
		t.Errorf("a failed start held on to port %d", st.Port)
	}
}

// missingPackage is a distribution name the index will not have. The astro-e2e
// prefix is not registered on PyPI and is not going to be; the suffix keeps a
// future squatter from turning this case green.
const missingPackage = "astro-e2e-no-such-package-8f3a1c"

// unresolvable adds a dependency that cannot resolve, so the next start fails
// in provisioning.
func unresolvable(t *testing.T, p *project) {
	t.Helper()
	path := filepath.Join(p.Dir, "pyproject.toml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	const anchor = "dependencies = ["
	i := bytes.Index(b, []byte(anchor))
	if i < 0 {
		t.Fatalf("no %q in the scaffolded manifest:\n%s", anchor, b)
	}
	j := i + len(anchor)
	out := string(b[:j]) + "'" + missingPackage + "==9.9.9', " + string(b[j:])
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// route is one entry of routes.json, the proxy's hostname table. Only the two
// fields this case asks about; the file carries more.
type route struct {
	Hostname   string `json:"hostname"`
	ProjectDir string `json:"projectDir"`
}

// routes reads the route table this project's ASTRO_HOME resolves against.
// Absent or empty is no routes rather than an error: a start that claimed
// nothing may never have created the file, which is the passing case here.
func routes(t *testing.T, p *project) []route {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(p.home, ".astro", "proxy", "routes.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading routes.json: %v", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil
	}
	var table []route
	if err := json.Unmarshal(b, &table); err != nil {
		t.Fatalf("routes.json is not a list of routes: %v\n%s", err, b)
	}
	return table
}
