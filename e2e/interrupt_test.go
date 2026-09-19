//go:build e2e && !windows

// Unix only, for the same reason main_signal_test.go is: there is no portable
// way to deliver SIGINT to another process on Windows, and syscall.Kill does
// not exist there. What is under test — cancel rather than kill — is what
// Ctrl-C produces on both, but only one of them can be provoked from a test.

package e2e

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// session is a command still running, so a case can interrupt it.
//
// Every other case here runs the CLI to completion and then asks what it did.
// That cannot express the question Ctrl-C asks, which is what happens to work
// already in flight — so this starts the binary, hands back a handle, and
// leaves the waiting to the caller.
type session struct {
	t    *testing.T
	args []string
	cmd  *exec.Cmd

	mu             sync.Mutex
	stdout, stderr strings.Builder

	done    chan struct{}
	waitErr error
}

// background starts `astro <args>` and returns without waiting for it.
//
// The child leads its own process group, and interrupt signals the group
// rather than the one process. That is what a terminal does — Ctrl-C goes to
// the foreground group — and it matters here because the interesting work is
// in the children: a start that is provisioning is really uv, and signaling
// only the parent would test a case that does not happen at a keyboard.
func (p *project) background(args ...string) *session {
	p.t.Helper()

	s := &session{t: p.t, args: args, done: make(chan struct{})}
	s.cmd = exec.Command(astroBin, args...)
	s.cmd.Dir = p.Dir
	s.cmd.Env = p.env(nil)
	s.cmd.Stdout = streamTo(&s.mu, &s.stdout)
	s.cmd.Stderr = streamTo(&s.mu, &s.stderr)
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := s.cmd.Start(); err != nil {
		p.t.Fatalf("starting `astro %s`: %v", strings.Join(args, " "), err)
	}
	go func() {
		s.waitErr = s.cmd.Wait()
		close(s.done)
	}()

	// However the case ends, the group does not outlive it. A test that fails
	// before its own interrupt would otherwise leave a start running against
	// the shared uv cache and a real Airflow behind it.
	p.t.Cleanup(func() {
		select {
		case <-s.done:
			return
		default:
		}
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		<-s.done
	})
	return s
}

// awaitStdout blocks until the running command has printed want, so a case can
// interrupt it in a known phase rather than at whatever moment the scheduler
// happened to reach.
func (s *session) awaitStdout(want string, within time.Duration) {
	s.t.Helper()
	deadline := time.After(within)
	for {
		s.mu.Lock()
		got := s.stdout.String()
		s.mu.Unlock()
		if strings.Contains(got, want) {
			return
		}
		select {
		case <-s.done:
			s.t.Fatalf("`astro %s` exited before printing %q\n%s",
				strings.Join(s.args, " "), want, s.snapshot())
		case <-deadline:
			s.t.Fatalf("`astro %s` did not print %q within %s\n%s",
				strings.Join(s.args, " "), want, within, s.snapshot())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// interrupt is Ctrl-C: SIGINT to the whole process group.
func (s *session) interrupt() {
	s.t.Helper()
	if err := syscall.Kill(-s.cmd.Process.Pid, syscall.SIGINT); err != nil {
		s.t.Fatalf("interrupting `astro %s`: %v", strings.Join(s.args, " "), err)
	}
}

// wait blocks for the command to end and reports what it did, in the shape the
// synchronous cases use.
func (s *session) wait(within time.Duration) *result {
	s.t.Helper()
	select {
	case <-s.done:
	case <-time.After(within):
		s.t.Fatalf("`astro %s` did not exit within %s of the interrupt\n%s",
			strings.Join(s.args, " "), within, s.snapshot())
	}

	s.mu.Lock()
	r := &result{t: s.t, Args: s.args, Stdout: s.stdout.String(), Stderr: s.stderr.String()}
	s.mu.Unlock()

	var exit *exec.ExitError
	switch {
	case s.waitErr == nil:
	case errors.As(s.waitErr, &exit):
		r.ExitCode = exit.ExitCode()
	default:
		s.t.Fatalf("waiting for `astro %s`: %v", strings.Join(s.args, " "), s.waitErr)
	}
	return r
}

func (s *session) snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("--- stdout\n%s\n--- stderr\n%s", s.stdout.String(), s.stderr.String())
}

// streamTo is an io.Writer appending under mu, because the test reads these
// buffers while the process is still writing to them.
func streamTo(mu *sync.Mutex, b *strings.Builder) *lockedWriter {
	return &lockedWriter{mu: mu, b: b}
}

type lockedWriter struct {
	mu *sync.Mutex
	b  *strings.Builder
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

// errorLine is the CLI's own error line out of a stderr stream that may also
// carry warnings and notices. Empty when there is none.
//
// Assertions about what the CLI reported belong on this line: a scan of the
// whole stream passes or fails on text from anything else that wrote there,
// which for a start includes the port-fallback notice and any proxy warning.
func errorLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "Error:") {
			return line
		}
	}
	return ""
}

// exitInterrupted is what a shell reports for a process ended by SIGINT, and
// what main.go returns for a canceled command.
const exitInterrupted = 130

// Ctrl-C while the environment is being built leaves nothing behind.
//
// The first of the two halves of "cancels, it doesn't kill": interrupted
// before anything is running, the command must clean up after itself rather
// than die mid-write. A record or a hostname claimed here outlives the
// keystroke and is only cleared by `--clean`.
//
// Tier 1: the interrupt lands during the uv sync, so no Airflow is ever
// launched. The sibling case below is the one that needs a real one.
func TestInterruptDuringTheSyncLeavesNothingBehind(t *testing.T) {
	tier(t, 1)
	needsUV(t)

	p := newProject(t)
	p.run("init", "--name", "syncstop").requireSuccess()

	s := p.background("local", "start")
	// uv has spoken, so the sync is under way and there is something to
	// interrupt. Interrupting before this raced the process to its first
	// instruction and tested nothing.
	s.awaitStdout("[uv]", 90*time.Second)
	s.interrupt()
	r := s.wait(60 * time.Second)

	if r.ExitCode != exitInterrupted {
		t.Errorf("exit = %d, want %d for an interrupted command\n%s", r.ExitCode, exitInterrupted, r.output())
	}

	// And it says it was interrupted rather than reporting a uv failure —
	// pkg/uv's interruptedError has the why. Asserted here as well as there
	// because this is where the words reach a person.
	//
	// "was interrupted" rather than "interrupted", and the project is not
	// named that either: it was, and since the CLI prints the project path and
	// the derived hostname on several paths, the assertion could have been
	// satisfied by the fixture's own name rather than by the message.
	//
	// The absence is checked on the error line rather than on the whole
	// stream, because other things legitimately write to stderr during a start
	// and one of them containing the word would fail a case about something
	// else.
	r.requireStderr("was interrupted")
	if line := errorLine(r.Stderr); strings.Contains(line, "uv sync failed") {
		t.Errorf("the error line still reports a failure for something the user asked for: %s", line)
	}

	// Read before `list` or `status`: the route store drops entries whose pid
	// is gone whenever a command opens it, so asking those first would clear a
	// leaked route and leave this asserting the pruner's work.
	if left := routes(t, p); len(left) != 0 {
		t.Errorf("an interrupted sync left %d route(s) claimed: %+v", len(left), left)
	}
	if _, listed := lineWith(p.run("local", "list", "--all").requireSuccess().Stdout, p.Dir); listed {
		t.Error("an interrupted sync left a record in `astro local list --all`")
	}
	if st := p.status(); st.State != "stopped" || st.PID != 0 || st.Port != 0 {
		t.Errorf("status after an interrupted sync = %+v, want stopped with no pid or port", st)
	}

	// Whether the next start recovers is the other half of the promise, and it
	// is not asserted here: proving it needs a start that finishes, which
	// needs an Airflow. TestAStartAfterAnInterruptedSyncSucceeds does it at
	// the tier that can afford one.
	//
	// `astro local check` looked like a cheap stand-in and is not. An
	// interrupt leaves a .venv holding a Python and no airflow, and check
	// resolves the project's interpreter when one exists rather than
	// provisioning over it — so it fails with "project environment is not
	// ready to check" where a start would rebuild. Asserting check here would
	// have pinned that as the promise.
}

// A start after an interrupted sync finishes the job.
//
// The rest of the promise: an interrupt leaves a half-built .venv, and what
// matters to whoever pressed the key is that nothing has to be cleaned up by
// hand. uv's sync is resumable, so the second start completes it and brings
// Airflow up — but that is a property of uv rather than something this repo
// arranges, which is why it is worth a case.
func TestAStartAfterAnInterruptedSyncSucceeds(t *testing.T) {
	tier(t, 2)

	p := newProject(t)
	p.run("init", "--name", "resumed").requireSuccess()

	s := p.background("local", "start")
	s.awaitStdout("[uv]", 90*time.Second)
	s.interrupt()
	s.wait(60 * time.Second)

	t.Cleanup(func() { p.run("local", "stop") })
	p.runSlow("local", "start").requireSuccess()

	if st := p.status(); st.State != "running" {
		t.Errorf("state after the second start = %q, want running", st.State)
	}
}

// Ctrl-C once Airflow is up leaves it up.
//
// The second half of "cancels, it doesn't kill", and the one that is a product
// decision rather than a cleanup: a start interrupted a moment before
// readiness must not throw away an instance that is nearly there. So the
// runtime is left running, recorded AND routed — the record is what `stop`
// reaps it through, and the route is what makes the named URL answer. An
// earlier revision of the engine registered the route only after the health
// wait, so an interrupt in this window left Airflow running and reachable only
// by port, with `astro local status` printing a hostname that 404s.
func TestInterruptDuringTheHealthWaitKeepsAirflow(t *testing.T) {
	tier(t, 2)

	p := newProject(t)
	p.run("init", "--name", "nearlyup").requireSuccess()
	t.Cleanup(func() { p.run("local", "stop") })

	s := p.background("local", "start")
	// The RECORD is the signal, not the route.
	//
	// Both are written between the launch and the health wait, so either would
	// put this case in the window it is about. The route was the first choice
	// and was the wrong one: the route is also what this case asserts, so
	// moving the registration — the very regression the assertion watches for
	// — moved the signal with it, and the case failed by timing out on a start
	// that had already finished rather than by noticing the missing route.
	// A signal has to be independent of what it is a signal for.
	awaitRecorded(t, p, 5*time.Minute)
	s.interrupt()
	r := s.wait(2 * time.Minute)

	if r.ExitCode != exitInterrupted {
		t.Errorf("exit = %d, want %d for an interrupted command\n%s", r.ExitCode, exitInterrupted, r.output())
	}

	// And the message says the runtime was left up. "context canceled" was
	// what this printed: the name of a Go value, describing the mechanism
	// rather than the outcome, with no way for the reader to tell that an
	// Airflow is now running on their machine.
	for _, want := range []string{"still starting", "astro local stop"} {
		r.requireStderr(want)
	}
	if line := errorLine(r.Stderr); strings.Contains(line, "context canceled") {
		t.Errorf("the error line names the mechanism instead of the outcome: %s", line)
	}

	// Still running, and still recorded: this is the instance the interrupt
	// deliberately did not throw away.
	st := p.status()
	if st.State != "running" {
		t.Fatalf("state after an interrupt in the health wait = %q, want running\n%s", st.State, r.output())
	}
	if st.PID == 0 || st.Port == 0 {
		t.Errorf("status = %+v, want a pid and a port", st)
	}
	if _, listed := lineWith(p.run("local", "list").requireSuccess().Stdout, p.Dir); !listed {
		t.Error("the interrupted start's Airflow is not in `astro local list`")
	}

	// And still routed. Without this the hostname status prints answers
	// nothing, which is the regression this window used to have.
	var routed bool
	for _, rt := range routes(t, p) {
		if rt.Hostname == st.Hostname {
			routed = true
		}
	}
	if !routed {
		t.Errorf("no route for %q, so the named URL status prints would not answer: %+v", st.Hostname, routes(t, p))
	}

	// And it finishes coming up. The record surviving is not the promise on its
	// own — `status` reports running from a live pid, so a record and a pid
	// that never serves anything would satisfy everything above while the
	// instance the interrupt was supposed to preserve was in fact lost. What
	// was nearly up has to actually arrive.
	if !acceptsWithin(st.Port, 4*time.Minute) {
		t.Fatalf("port %d never opened, so the interrupt did throw away the instance it left recorded", st.Port)
	}
	if code := get(t, st.Port); code != http.StatusOK {
		t.Errorf("GET / on the port the interrupted start left recorded = %d, want 200", code)
	}

	// `stop` reaps what the interrupt left, which is what makes leaving it
	// running a decision rather than a leak.
	p.runSlow("local", "stop").requireSuccess()
	if stopped := p.status(); stopped.State != "stopped" {
		t.Errorf("state after stop = %q, want stopped", stopped.State)
	}
}

// acceptsWithin polls until a port takes a connection, which Airflow's does
// only once its own startup has finished — the interrupt landed while that was
// still in progress.
//
// A bare dial rather than get: this runs until it succeeds, and get logs every
// refusal, so against a cold Airflow the transcript would be a few hundred
// lines of connection-refused before the one line that matters. The HTTP
// question is asked once, after this says there is something listening.
func acceptsWithin(port int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

// awaitRecorded blocks until the project has a running record, which a start
// writes once it has launched Airflow and before it waits for health.
func awaitRecorded(t *testing.T, p *project, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if p.status().State == "running" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("nothing was recorded within %s, so the start never reached the health wait", within)
}
