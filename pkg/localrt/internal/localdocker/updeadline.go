package localdocker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Why `compose up --detach` needs a deadline of its own.
//
// It does not return when the containers are started. The Airflow services
// declare `depends_on: db-migration: condition: service_completed_successfully`,
// so compose starts postgres and the migration and then waits, inside the up,
// for the migration to exit 0. When it never does — postgres crash-looping
// under `restart: unless-stopped` on a data directory it refuses, and the
// migration's entrypoint waiting on it forever — the up never returns either,
// and the health timeout, which was only applied after it, never started.
// Measured: a start sat on "db-migration Waiting" for as long as anyone
// watched.
//
// So the health timeout is one budget for the whole of "Airflow comes up": the
// up runs under it, and the health wait gets what the up left. Pulling and
// creating the containers happen first and outside it, as their own step,
// because a cold pull of a 1.3 GB runtime image on a slow link is not the
// failure the knob is about, and charging it to the budget would fail a first
// start that was making progress the whole time.

// minHealthWait is the least the health wait is given, so an up that finished
// at the very end of the budget still gets a probe or two rather than a
// timeout that never asked.
const minHealthWait = time.Second

// awaitHealth waits for Airflow to answer with what the up left of the budget.
//
// The wait is told the time it actually has, so its message reports that, and
// a timeout says where the rest of the budget went: "timed out after 12s" from
// a start given 45s otherwise reads as the setting not taking.
func (e *Engine) awaitHealth(ctx context.Context, urls []string, remaining time.Duration) error {
	wait := max(remaining, minHealthWait)
	if wait > time.Second {
		wait = wait.Round(time.Second)
	}
	err := e.health(ctx, urls, wait)
	if err != nil && errors.Is(err, ErrHealthTimeout) && wait < e.healthTimeout {
		return fmt.Errorf("%w (what was left of the %s budget: the containers took %s of it to start)",
			err, e.healthTimeout, (e.healthTimeout - remaining).Round(time.Second))
	}
	return err
}

// ErrServiceFailed is a start whose containers did not come up because one of
// them kept failing, as distinct from one that was merely slow.
var ErrServiceFailed = rt.ErrServiceFailed

// upFailure is what upTimedOut found out about an up that ran out the budget.
type upFailure struct {
	// err is the error to report.
	err error
	// failing reports that a service kept failing, rather than the up being
	// slow.
	failing bool
	// revision is the migration revision the stuck services' logs say Airflow
	// could not find, or "" (see captureFailureLogs).
	revision string
	// logsRead reports that the stuck services' logs were read, so the
	// rollback need not read them again.
	logsRead bool
}

// upTimedOut diagnoses an up that ran out the budget, from what compose says
// about the project's containers.
//
// Two answers, because they call for opposite advice. A service that kept
// exiting non-zero — postgres crash-looping on a data directory it refuses —
// is ErrServiceFailed: no budget would have been enough, so the error must not
// say to wait longer, and --output json must not say Airflow may still be
// coming up. Anything else — the migration still running, nothing failed — is
// ErrHealthTimeout: slow, and a longer budget is the remedy.
//
// Diagnosis is best effort and bounded: ctx must not be the expired up
// context, or neither query could even be spawned.
func (e *Engine) upTimedOut(ctx context.Context, up *composeLine) upFailure {
	ctx, cancel := context.WithTimeout(ctx, logCaptureTimeout)
	defer cancel()

	var (
		detail strings.Builder
		out    upFailure
	)
	if services, err := e.sampleServices(ctx, *up); err == nil {
		var pending, stuck []string
		pending, stuck, out.failing = pendingServices(services)
		if len(pending) > 0 {
			detail.WriteString(": " + strings.Join(pending, "; "))
		}
		if len(stuck) > 0 {
			var tails map[string]string
			tails, out.revision, out.logsRead = e.serviceTails(ctx, up, stuck)
			for _, svc := range stuck {
				if tail := tails[svc]; tail != "" {
					fmt.Fprintf(&detail, ". Last output from %s: %s", svc, tail)
				}
			}
		}
	}
	if out.failing {
		out.err = fmt.Errorf("%w, so the start gave up after %s%s", ErrServiceFailed, e.healthTimeout, detail.String())
	} else {
		out.err = fmt.Errorf("%w after %s, before the containers finished starting%s",
			ErrHealthTimeout, e.healthTimeout, detail.String())
	}
	return out
}

// serviceSamples and serviceSampleGap bound how many times, and how far
// apart, an up with nothing failing is looked at again before it is called
// slow. A var so a test need not wait out the gap.
var (
	serviceSamples   = 4
	serviceSampleGap = 500 * time.Millisecond
)

// sampleServices is composeServices asked until something is seen failing, up
// to serviceSamples times, keeping for each service its worst sighting.
//
// Once is not enough on podman. It restarts a crashed container at once, so
// a crash loop is "running" with exit code 1 or "stopped" with 1 at most
// instants — but "stopped" with 0 at some, mid-transition, which is
// indistinguishable from a service still on its way. Measured: a start whose
// one look landed there reported postgres as slow and advised waiting longer.
// A loop that restarts several times a second shows its exit code within a
// couple of looks; docker shows "restarting" for the whole backoff, so it is
// caught by the first.
func (e *Engine) sampleServices(ctx context.Context, up composeLine) ([]composeService, error) {
	worst := map[string]composeService{}
	var order []string
	for i := range serviceSamples {
		if i > 0 {
			select {
			case <-ctx.Done():
				return mergedServices(worst, order), nil
			case <-time.After(serviceSampleGap):
			}
		}
		services, err := e.composeServices(ctx, up)
		if err != nil {
			if i == 0 {
				return nil, err
			}
			break
		}
		for _, s := range services {
			prev, seen := worst[s.Service]
			if !seen {
				order = append(order, s.Service)
			}
			if !seen || (failingState(s) && !failingState(prev)) {
				worst[s.Service] = s
			}
		}
		if _, _, failing := pendingServices(mergedServices(worst, order)); failing {
			break
		}
	}
	return mergedServices(worst, order), nil
}

func mergedServices(worst map[string]composeService, order []string) []composeService {
	out := make([]composeService, 0, len(order))
	for _, name := range order {
		out = append(out, worst[name])
	}
	return out
}

// failingState is pendingServices' test for a failing service, on one entry.
func failingState(s composeService) bool {
	return s.ExitCode != 0 || s.State == "restarting" || s.State == "dead"
}

// composeService is the part of one `compose ps --format json` entry this
// reads.
type composeService struct {
	Service  string
	State    string
	ExitCode int
}

// composeServices lists the project's containers, stopped ones included.
//
// --format json has been spelled two ways: one JSON array (compose before
// 2.21) and one object per line (after). Both are read.
func (e *Engine) composeServices(ctx context.Context, up composeLine) ([]composeService, error) {
	out, err := e.cmd.Output(ctx, up.conn.env, up.conn.bin, "compose", "-p", up.name, "ps", "-a", "--format", "json")
	if err != nil {
		return nil, err
	}
	out = bytes.TrimSpace(out)
	var services []composeService
	if bytes.HasPrefix(out, []byte("[")) {
		if err := json.Unmarshal(out, &services); err != nil {
			return nil, err
		}
		return services, nil
	}
	for line := range bytes.Lines(out) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var s composeService
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, err
		}
		services = append(services, s)
	}
	return services, nil
}

// migrationService is the one-shot service the components wait on.
const migrationService = "db-migration"

// pendingServices describes what the up was still waiting on, names the
// services whose own output is the likeliest explanation, and reports whether
// any of them is failing rather than slow.
//
// A service is done when it exited 0: the one-shot migration. It is waiting on
// a dependency when it is only created, which says nothing worth quoting, and
// in progress when it is running. It is failing when it has a non-zero exit
// code whatever its state, or is restarting or dead — and its logs say why.
//
// By exit code rather than a list of failure states, because the engines do
// not agree on how a crash loop looks. Docker shows the container
// "restarting" between attempts. Podman restarts it at once and has no such
// state: sampled mid-loop, `podman compose ps -a` reported it "running" with
// the last exit code (1) about half the time and "stopped" the rest, with
// code 1 or, mid-transition, 0. A list naming docker's spellings missed all
// of those, and quoted the migration waiting on postgres instead of postgres.
// Hence "running" with a non-zero exit code is failing: up again, but only
// after crashing. And "stopped" with 0 is not: it is podman's word for a
// finished one-shot as well as for that transition, so it counts as done for
// the migration and as still in progress for anything else.
func pendingServices(services []composeService) (pending, stuck []string, failing bool) {
	slices.SortFunc(services, func(a, b composeService) int { return strings.Compare(a.Service, b.Service) })
	var waiting, inProgress, slow []string
	for _, s := range services {
		switch {
		case s.ExitCode == 0 && (s.State == "exited" || (s.State == "stopped" && s.Service == migrationService)):
			// Done.
		case s.State == "created":
			waiting = append(waiting, s.Service)
		case s.ExitCode == 0 && s.State != "restarting" && s.State != "dead":
			slow = append(slow, s.Service)
			if s.State == "running" {
				inProgress = append(inProgress, s.Service)
			} else {
				inProgress = append(inProgress, s.Service+" ("+s.State+")")
			}
		default:
			pending = append(pending, s.Service+" "+describeFailure(s))
			stuck = append(stuck, s.Service)
		}
	}
	failing = len(stuck) > 0
	if len(waiting) > 0 {
		pending = append(pending, "never started, waiting on a dependency: "+strings.Join(waiting, ", "))
	}
	if !failing && len(waiting) > 0 && len(slow) > 0 {
		// Nothing failed outright and something is held back, so what is
		// running is taking too long. There are at most two (the database and
		// the migration) before the components start. With nothing held back,
		// a running service is just running, and naming it would accuse it.
		pending = append(pending, "still running: "+strings.Join(inProgress, ", "))
		stuck = slow
	}
	return pending, stuck, failing
}

// describeFailure says how a failing service looks, with its exit code when
// it has one: the code is the same fact on either engine, where the state is
// not.
func describeFailure(s composeService) string {
	switch {
	case s.State == "exited":
		return fmt.Sprintf("exited (%d)", s.ExitCode)
	case s.State == "running":
		return fmt.Sprintf("keeps exiting (last exit code %d)", s.ExitCode)
	case s.ExitCode != 0:
		return fmt.Sprintf("is %s (last exit code %d)", s.State, s.ExitCode)
	}
	return "is " + s.State
}

// serviceTailLines bounds the output quoted per service: enough for a FATAL
// line and its context, short enough that the error stays an error.
// serviceTailRead is how much is read to find them: a crash loop prints the
// same line once per restart, and five copies of it say less than one.
const (
	serviceTailLines = 5
	serviceTailRead  = 50
)

// serviceTails reads the stuck services' recent output in one `compose logs`,
// and returns each one's last few distinct lines joined into one line, the
// migration revision those lines say Airflow could not find, and whether the
// logs could be read at all.
//
// One call for every service rather than one each: on a wedged daemon each
// call can take the whole capture budget. Split by compose's per-container
// prefix, the same parse Logs uses.
func (e *Engine) serviceTails(ctx context.Context, up *composeLine, services []string) (tails map[string]string, revision string, read bool) {
	args := append([]string{"compose", "-p", up.name, "logs", "--no-color", "--tail", strconv.Itoa(serviceTailRead)}, services...)
	out, err := e.cmd.Output(ctx, up.conn.env, up.conn.bin, args...)
	if err != nil {
		return nil, "", false
	}
	byService := map[string][]string{}
	for line := range strings.Lines(string(out)) {
		l := parseLogLine(strings.TrimRight(line, "\r\n"), e.now)
		text := strings.TrimSpace(l.Text)
		if text == "" {
			continue
		}
		if revision == "" {
			revision, _ = airflowrt.UnknownMigrationRevision(text)
		}
		byService[l.Component] = append(byService[l.Component], text)
	}
	tails = make(map[string]string, len(byService))
	for svc, lines := range byService {
		tails[svc] = strings.Join(lastDistinct(lines, serviceTailLines), " | ")
	}
	return tails, revision, true
}

// lastDistinct is the last n distinct lines, in the order they were printed,
// each at its LAST occurrence. Keeping the first occurrence instead placed a
// crash loop's repeated FATAL at its earliest position, where the cut to n
// could drop it in favor of older, less useful lines.
func lastDistinct(lines []string, n int) []string {
	var picked []string
	for i := len(lines) - 1; i >= 0 && len(picked) < n; i-- {
		if !slices.Contains(picked, lines[i]) {
			picked = append(picked, lines[i])
		}
	}
	slices.Reverse(picked)
	return picked
}
