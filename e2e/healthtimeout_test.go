//go:build e2e && !windows

package e2e

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// impatientHealth is a health budget nothing can beat, so a start reaches its
// timeout in the time it takes to launch rather than in five minutes.
//
// The whole reason ASTRO_LOCAL_HEALTH_TIMEOUT exists as something a caller can
// set: what a start leaves behind when it never becomes healthy was, until it
// did, reachable only by waiting out the default — which is why neither of the
// two cases below existed while the plan asked for them.
const impatientHealth = "1ms"

// startImpatiently runs a start whose health wait cannot succeed.
func startImpatiently(p *project, args ...string) *result {
	p.t.Helper()
	return p.runBounded(slowCommandTimeout,
		map[string]string{"ASTRO_LOCAL_HEALTH_TIMEOUT": impatientHealth},
		append([]string{"local", "start"}, args...)...)
}

// A standalone start that never becomes healthy is taken down completely.
//
// The other half of the interrupt case. An interrupt leaves Airflow running on
// purpose, because somebody asked to stop waiting and the instance was nearly
// up; a start that gave up on its own has no such instance to preserve, so the
// process is killed and the record and route go with it. Leaving them is the
// failure that matters — a record saying running with nothing behind it, and a
// hostname pointing at a port this start just released.
func TestAStandaloneStartThatNeverBecomesHealthyIsTornDown(t *testing.T) {
	tier(t, 2)

	p := newProject(t)
	p.run("init", "--name", "impatient").requireSuccess()
	t.Cleanup(func() { p.runSlow("local", "stop") })

	r := startImpatiently(p).requireFailure()

	// The message says what ran out and where to look, and names the knob —
	// which is the only place a reader learns the wait is theirs to change.
	for _, want := range []string{"timed out", "ASTRO_LOCAL_HEALTH_TIMEOUT"} {
		if !strings.Contains(r.Stderr, want) {
			t.Errorf("stderr does not carry %q\n%s", want, r.output())
		}
	}
	if !strings.Contains(r.Stderr, "airflow.log") {
		t.Errorf("the message does not say where the output went\n%s", r.output())
	}

	// Read before `list` or `status`, as the sibling cases do: the route store
	// prunes dead entries whenever a command opens it, so asking those first
	// would clear a leaked route and leave this asserting the pruner's work.
	if left := routes(t, p); len(left) != 0 {
		t.Errorf("a timed-out start left %d route(s) claimed: %+v", len(left), left)
	}
	if _, listed := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir); listed {
		t.Error("a timed-out start left a record in `astro local list --all`")
	}
	if st := p.status(); st.State != "stopped" || st.PID != 0 || st.Port != 0 {
		t.Errorf("status after a timed-out start = %+v, want stopped with no pid or port", st)
	}

	// And the process is actually gone, asked of the operating system rather
	// than of the CLI.
	//
	// Everything above reads what the teardown WROTE, and the teardown removes
	// the record and the route whether or not the kill worked — so dropping
	// the kill leaves an orphaned Airflow holding a port with nothing able to
	// name it, and every assertion so far still passes. Measured: it does.
	// This is the standalone equivalent of asking docker whether the
	// containers are really gone.
	// Waited for, because the teardown is asynchronous by design: the CLI
	// signals the process group and returns, while the supervisor traps that
	// signal on purpose and waits on its child for up to its own grace period.
	// Asserting the instant the CLI exits races that, and runtime_test.go
	// already calls a killed group's teardown the one genuinely asynchronous
	// thing in this suite.
	waitFor(t, "the process group to go", func() bool {
		return processesUnder(t, p.Dir) == 0
	})
}

// processesUnder counts running processes whose command line mentions dir.
//
// The project directory is a fresh temp path per case, so it appears in no
// other process on the machine — which makes it a precise filter where a
// process name would not be.
func processesUnder(t *testing.T, dir string) int {
	t.Helper()
	// Quoted: pgrep -f takes an extended regular expression, not a literal, and
	// a temp path carrying a "." or a "+" would match more than itself — or,
	// with a bracket, fail to compile and exit 2, which the handling below
	// would report as an error rather than an answer.
	out, err := exec.CommandContext(t.Context(), "pgrep", "-f", regexp.QuoteMeta(dir)).Output()
	if err != nil {
		// pgrep exits 1 when nothing matched, which is the answer here rather
		// than a failure.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return 0
		}
		t.Fatalf("listing processes under %s: %v", dir, err)
	}
	return len(strings.Fields(string(out)))
}

// A docker start that never becomes healthy keeps its containers, on purpose.
//
// The opposite of the standalone case above, and the difference is the point:
// compose containers carry on coming up after the CLI stops watching, so what
// the timeout means there is "not yet" rather than "failed". The message says
// so, the record and route stay for `logs`, `status` and `stop` to work
// against, and stop is what ends it.
//
// A regression either way is invisible without this: tearing the containers
// down would throw away an Airflow that was seconds from ready, and dropping
// only the record would leave five containers holding a port with nothing able
// to name them.
func TestADockerStartThatNeverBecomesHealthyKeepsItsContainers(t *testing.T) {
	tier(t, 3)

	p := dockerProject(t, "impatientdk")
	needsDocker(t, p)

	r := startImpatiently(p, "--docker").requireFailure()

	for _, want := range []string{"timed out", "keep starting", "ASTRO_LOCAL_HEALTH_TIMEOUT"} {
		if !strings.Contains(r.Stderr, want) {
			t.Errorf("stderr does not carry %q\n%s", want, r.output())
		}
	}

	// Still recorded, so the commands that read the record still work.
	st := p.status()
	if st.State != "running" {
		t.Fatalf("state after a timed-out docker start = %q, want running\n%s", st.State, r.output())
	}
	if _, listed := lineWith(p.run("local", "list").requireSuccess().Stdout, p.Dir); !listed {
		t.Error("the containers are up but the project is not in `astro local list`")
	}

	// And still routed: the named URL has to answer for a project the CLI is
	// telling its user to go and watch.
	var routed bool
	for _, rt := range routes(t, p) {
		if rt.Hostname == st.Hostname {
			routed = true
		}
	}
	if !routed {
		t.Errorf("no route for %q, so the URL status prints would not answer: %+v", st.Hostname, routes(t, p))
	}

	// Running, not merely present. containersFor counts exited containers on
	// purpose — its sibling case watches for a stop that degraded to a stop —
	// so asking it here would have accepted a stack that came up and died as
	// evidence that the containers "keep starting in the background", which is
	// the one thing this case is about.
	project := composeProject(t, p)
	if running := runningContainersFor(t, project); len(running) == 0 {
		t.Fatalf("no RUNNING containers for %s: the timeout took down what it said it was leaving up", project)
	}

	// Stop is what ends it, which is what makes leaving it running a decision
	// rather than a leak.
	p.runSlow("local", "stop", "--clean").requireSuccess()
	if left := containersFor(t, project); len(left) != 0 {
		t.Errorf("stop left %d container(s) behind: %v", len(left), left)
	}
	waitFor(t, "the port to be released", func() bool {
		return portClosed(t, st.Port)
	})
}
