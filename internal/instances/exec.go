package instances

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// execTimeout bounds one run of an auth helper. It is generous because a
// helper may open a browser, hit an SSO endpoint, or unlock a keychain, and
// bounded because a helper that hangs would otherwise hang the command with no
// explanation at all.
const execTimeout = 30 * time.Second

// execStderrInError is how much of a failing helper's stderr reaches the
// message. Enough to see what it complained about, short enough that a helper
// dumping a stack trace does not bury the command that produced it.
const execStderrInError = 400

// maxHelperOutput bounds what is kept from each of the helper's streams.
const maxHelperOutput = 8 << 10

// execWaitDelay is how long a killed helper's output pipes are given to close
// before they are taken away. Short: by then the helper is already dead and
// only whatever it left behind is still holding them.
const execWaitDelay = 2 * time.Second

// execCredentials runs the link's helper and reads a token from its stdout —
// the kubectl exec-plugin pattern, and the escape hatch that lets the auth
// menu stay closed: an Airflow behind a front door nobody here has heard of is
// one small program away.
//
// The argv runs directly, never through a shell, which is why the manifest
// spells it as an array. The environment is inherited whole: a helper's own
// configuration (AWS_PROFILE, a vault address, a kubeconfig) reaches it the
// way it would from the user's own prompt.
//
// The token is held for the run and re-read on refresh, so a long command that
// outlives a short-lived token gets a fresh one without running the helper
// once per request.
func execCredentials(i Instance) (airflowapi.CredentialSource, func(context.Context) error, error) {
	argv := i.Link.Auth.Command
	if len(argv) == 0 {
		return nil, nil, fmt.Errorf("instance %q declares the exec method with no command to run", i.Name)
	}
	h := &execHelper{instance: i.Name, argv: argv, timeout: execTimeout}
	return h.credentials, h.refresh, nil
}

type execHelper struct {
	instance string
	argv     []string
	timeout  time.Duration

	mu    sync.Mutex
	token string
	// running is the run in flight, closed when it finishes. It is how a second
	// caller waits without holding the lock across a subprocess: waiting on a
	// mutex cannot be given up on, and a caller whose context is already dying
	// should not be pinned behind a helper that is about to be killed anyway.
	// airflowapi.Client.detect is the same shape for the same reason.
	running *execRun
}

// execRun is one run of the helper: the callers waiting on it share its answer.
type execRun struct {
	done  chan struct{}
	token string
	err   error
}

func (h *execHelper) credentials(ctx context.Context) (scheme, value string, err error) {
	token, err := h.held(ctx)
	if err != nil {
		return "", "", err
	}
	return airflowapi.BearerToken(token)(ctx)
}

// held hands back the token, running the helper once if nobody has.
func (h *execHelper) held(ctx context.Context) (string, error) {
	h.mu.Lock()
	// run never hands back an empty token, so a token in hand is the whole
	// "already asked" flag.
	if h.token != "" {
		defer h.mu.Unlock()
		return h.token, nil
	}
	if running := h.running; running != nil {
		h.mu.Unlock()
		select {
		case <-running.done:
			return running.token, running.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	running := &execRun{done: make(chan struct{})}
	h.running = running
	h.mu.Unlock()

	running.token, running.err = h.run(ctx)

	h.mu.Lock()
	if running.err == nil {
		h.token = running.token
	}
	h.running = nil
	h.mu.Unlock()
	close(running.done)
	return running.token, running.err
}

func (h *execHelper) refresh(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.token = ""
	return nil
}

// run executes the helper and reads its answer. Every way it can go wrong
// quotes the command, because the reader's next move is to run it themselves.
func (h *execHelper) run(parent context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(parent, h.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.argv[0], h.argv[1:]...) //nolint:gosec // the command is the user's own, declared in their manifest
	// Bounded: a token is one line and the message quotes at most a few
	// hundred bytes of complaint, so a helper that decides to print its whole
	// log costs a few kilobytes rather than however much it felt like.
	stdout := &boundedBuffer{limit: maxHelperOutput}
	stderr := &boundedBuffer{limit: maxHelperOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	// Without this the bound is not a bound. Killing the helper does not close
	// the pipes a grandchild inherited, and cmd.Run waits for them: a helper
	// that backgrounds anything — an SSO daemon, a keychain agent — holds the
	// command open for that child's whole life, timeout or no timeout.
	cmd.WaitDelay = execWaitDelay

	err := cmd.Run()
	// A helper that exited cleanly and left a child holding the pipes comes
	// back as ErrWaitDelay. The helper did its job; what it left running is not
	// this command's business.
	if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success() {
		err = nil
	}
	if deadline := ctx.Err(); errors.Is(deadline, context.DeadlineExceeded) {
		return "", fmt.Errorf("the auth command for instance %q did not finish within %s: %s%s",
			h.instance, h.waited(parent), h.quoted(), stderrDetail(stderr.String()))
	}
	if err != nil {
		return "", fmt.Errorf("the auth command for instance %q failed: %s: %w%s", h.instance, h.quoted(), err, stderrDetail(stderr.String()))
	}

	token := trimToken(stdout.String())
	switch {
	case token == "":
		return "", fmt.Errorf("the auth command for instance %q printed no token: %s%s", h.instance, h.quoted(), stderrDetail(stderr.String()))
	case strings.ContainsAny(token, "\r\n"):
		// A helper that prints a token plus a log line, or a whole JSON
		// document, is a helper being asked for the wrong thing. Sending the
		// first line and hoping would produce a 401 nobody could explain.
		return "", fmt.Errorf("the auth command for instance %q printed %d lines, and a token is one: %s",
			h.instance, len(strings.Split(token, "\n")), h.quoted())
	}
	return token, nil
}

// waited reports the deadline that actually expired. The helper's own bound is
// usually it, but a caller with a shorter deadline of its own gets there first,
// and telling that reader their helper had 30 seconds would send them looking
// for a slow helper instead of a short command timeout.
func (h *execHelper) waited(parent context.Context) string {
	deadline, ok := parent.Deadline()
	if !ok {
		return h.timeout.String()
	}
	if left := time.Until(deadline); left < h.timeout {
		return "the time this command had left"
	}
	return h.timeout.String()
}

// utf8BOM is the byte-order mark a helper written in PowerShell or .NET puts
// in front of its output. It is invisible in a terminal, and a 401 nobody can
// explain if it reaches the wire.
const utf8BOM = "\xef\xbb\xbf"

// trimToken strips the whitespace around a helper's answer, and that mark.
func trimToken(stdout string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stdout), utf8BOM))
}

// quoted spells the argv the way it would be typed, so the message is a
// command the reader can run.
func (h *execHelper) quoted() string {
	parts := make([]string, len(h.argv))
	for i, arg := range h.argv {
		if strings.ContainsAny(arg, " \t\"'") {
			parts[i] = fmt.Sprintf("%q", arg)
			continue
		}
		parts[i] = arg
	}
	return strings.Join(parts, " ")
}

// stderrDetail appends what the helper complained about, when it complained.
func stderrDetail(stderr string) string {
	trimmed := strings.TrimSpace(stderr)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > execStderrInError {
		trimmed = trimmed[:execStderrInError] + "..."
	}
	return "\n      it said: " + trimmed
}

// boundedBuffer keeps the first limit bytes written to it and drops the rest,
// so a helper that never stops printing cannot grow the CLI's memory while it
// runs.
type boundedBuffer struct {
	limit int
	buf   bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			p = p[:room]
		}
		b.buf.Write(p)
	}
	// The whole write is reported as accepted: the helper is not at fault for
	// the cap and a short write would end it with a broken pipe.
	return len(p), nil
}

func (b *boundedBuffer) String() string { return b.buf.String() }
