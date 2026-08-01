package instances

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// These cover the ways an auth helper misbehaves that a helper author does not
// think of and a CLI still has to survive. They need a real shell script
// because the misbehavior is about processes — a background child, a partial
// write, a byte-order mark — not about Go.

func shellHelper(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("these cases are about POSIX process behavior")
	}
	path := filepath.Join(t.TempDir(), "helper.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestExecTimeoutIsAWallClockBound: killing the helper does not close the pipes
// a grandchild inherited, and cmd.Run waits for them. Without WaitDelay a 30s
// bound is 30s plus however long the grandchild feels like living.
func TestExecTimeoutIsAWallClockBound(t *testing.T) {
	h := &execHelper{
		deployment: "prod",
		argv:       []string{shellHelper(t, "sleep 30")},
		timeout:    200 * time.Millisecond,
	}
	start := time.Now()
	_, err := h.run(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("the bound did not apply")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("a %s bound took %s to give up", h.timeout, elapsed.Round(10*time.Millisecond))
	}
}

// TestExecHelperMayLeaveAChildBehind: a helper that starts an agent and exits
// is doing its job. Waiting for that agent's whole life would hang every
// command the helper serves.
func TestExecHelperMayLeaveAChildBehind(t *testing.T) {
	h := &execHelper{
		deployment: "prod",
		argv:       []string{shellHelper(t, "sleep 30 &\nprintf 'tok\\n'\nexit 0")},
		timeout:    30 * time.Second,
	}
	start := time.Now()
	token, err := h.run(context.Background())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("a helper that exited cleanly failed: %v", err)
	}
	if token != "tok" {
		t.Fatalf("token = %q", token)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("the helper exited at once and the command waited %s for its background child",
			elapsed.Round(10*time.Millisecond))
	}
}

// TestExecTokenShapes: what a helper prints, and what is read from it.
func TestExecTokenShapes(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		token string
	}{
		{"trailing newline", `printf 'tok\n'`, "tok"},
		{"windows line ending", `printf 'tok\r\n'`, "tok"},
		{"carriage return only", `printf 'tok\r'`, "tok"},
		{"padded", `printf '  tok  \n'`, "tok"},
		{"no trailing newline", `printf 'tok'`, "tok"},
		// PowerShell and .NET write this in front of their output. It is
		// invisible in a terminal and a 401 nobody can explain on the wire.
		{"byte-order mark", `printf '\357\273\277tok\n'`, "tok"},
		// A token with a space is legal and must not be split or trimmed
		// inside.
		{"inner space", `printf 'tok with space\n'`, "tok with space"},
		// Chatter on stderr is not a failure. Plenty of helpers warn.
		{"stderr on success", `printf 'renewing soon\n' >&2; printf 'tok\n'`, "tok"},
	}
	for _, tc := range cases {
		h := &execHelper{deployment: "prod", argv: []string{shellHelper(t, tc.body)}, timeout: 10 * time.Second}
		token, err := h.run(context.Background())
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if token != tc.token {
			t.Errorf("%s: token = %q, want %q", tc.name, token, tc.token)
		}
	}
}

// TestExecNonzeroExitBeatsATokenOnStdout: a helper that printed something and
// then failed did not succeed, and using what it printed would send a token it
// disowned.
func TestExecNonzeroExitBeatsATokenOnStdout(t *testing.T) {
	h := &execHelper{deployment: "prod", argv: []string{shellHelper(t, `printf 'tok\n'; exit 3`)}, timeout: 10 * time.Second}
	if _, err := h.run(context.Background()); err == nil {
		t.Fatal("a helper that exited 3 was taken at its word")
	}
}

// TestExecReportsTheDeadlineThatActuallyExpired: a command with less time left
// than the helper's own bound is the one that ran out, and saying "30s" would
// send the reader hunting a slow helper.
func TestExecReportsTheDeadlineThatActuallyExpired(t *testing.T) {
	h := &execHelper{deployment: "prod", argv: []string{shellHelper(t, "sleep 30")}, timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := h.run(ctx)
	if err == nil {
		t.Fatal("the parent deadline did not end the run")
	}
	if strings.Contains(err.Error(), "30s") {
		t.Fatalf("err = %v, want it to name the deadline that expired, not the helper's own", err)
	}
}

// TestExecRunsOnceUnderConcurrentCallers: the credential source is shared, and
// a helper that prompts must not prompt once per request.
func TestExecRunsOnceUnderConcurrentCallers(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "runs")
	h := &execHelper{
		deployment: "prod",
		argv:       []string{shellHelper(t, "printf x >> "+counter+"\nsleep 0.2\nprintf 'tok\\n'")},
		timeout:    10 * time.Second,
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := h.credentials(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	runs, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("the helper ran %d times, want once for the run", len(runs))
	}
}

// TestExecMissingProgramSaysSo: a typo in the manifest, which is the most
// likely thing to be wrong here.
func TestExecMissingProgramSaysSo(t *testing.T) {
	h := &execHelper{deployment: "prod", argv: []string{"no-such-astro-helper-xyz"}, timeout: time.Second}
	_, err := h.run(context.Background())
	if err == nil {
		t.Fatal("a missing program resolved")
	}
	if !strings.Contains(err.Error(), "no-such-astro-helper-xyz") || !strings.Contains(err.Error(), `"prod"`) {
		t.Fatalf("err = %v, want it to name the command and the instance", err)
	}
}
