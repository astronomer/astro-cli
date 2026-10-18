package localdocker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
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

// upTimedOut is the error for an up that ran out the budget: ErrHealthTimeout,
// so every caller that classifies the health timeout — `--output json`'s
// problem kind, the CLI's advice naming the variable — classifies this one the
// same, with what was still pending and what the stuck service last said.
//
// Diagnosis is best effort and bounded: ctx must not be the expired up
// context, or neither query could even be spawned.
func (e *Engine) upTimedOut(ctx context.Context, up *composeLine) error {
	ctx, cancel := context.WithTimeout(ctx, logCaptureTimeout)
	defer cancel()

	var detail strings.Builder
	if services, err := e.composeServices(ctx, *up); err == nil {
		pending, stuck := pendingServices(services)
		if len(pending) > 0 {
			detail.WriteString(": " + strings.Join(pending, "; "))
		}
		for _, svc := range stuck {
			if tail := e.serviceTail(ctx, up, svc); tail != "" {
				fmt.Fprintf(&detail, ". Last output from %s: %s", svc, tail)
			}
		}
	}
	return fmt.Errorf("%w after %s, before the containers finished starting%s",
		ErrHealthTimeout, e.healthTimeout, detail.String())
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

// pendingServices describes what the up was still waiting on, and names the
// services whose own output is the likeliest explanation.
//
// A service is done when it is running or exited 0 (the one-shot migration).
// Of the rest, one that is restarting or exited non-zero is failing, and its
// logs say why; one still running that others wait on is in progress, and its
// logs are the next best thing; one only created is waiting on a dependency
// and has said nothing.
func pendingServices(services []composeService) (pending, stuck []string) {
	slices.SortFunc(services, func(a, b composeService) int { return strings.Compare(a.Service, b.Service) })
	var waiting, running []string
	for _, s := range services {
		switch {
		case s.State == "restarting" || s.State == "dead" || (s.State == "exited" && s.ExitCode != 0):
			pending = append(pending, fmt.Sprintf("%s is %s", s.Service, describeState(s)))
			stuck = append(stuck, s.Service)
		case s.State == "created":
			waiting = append(waiting, s.Service)
		case s.State == "running":
			running = append(running, s.Service)
		}
	}
	if len(waiting) == 0 {
		// Nothing is held back, so whatever compose was waiting for is not a
		// dependency; the failing services, if any, are the whole story.
		return pending, stuck
	}
	pending = append(pending, "never started, waiting on a dependency: "+strings.Join(waiting, ", "))
	if len(stuck) == 0 {
		// Nothing failed outright, so something running is taking too long.
		// Every running service is a candidate; there are at most two (the
		// database and the migration) before the components start.
		stuck = running
		if len(running) > 0 {
			pending = append(pending, "still running: "+strings.Join(running, ", "))
		}
	}
	return pending, stuck
}

func describeState(s composeService) string {
	if s.State == "exited" {
		return fmt.Sprintf("exited with code %d", s.ExitCode)
	}
	return s.State
}

// serviceTailLines bounds the output quoted per service: enough for a FATAL
// line and its context, short enough that the error stays an error.
// serviceTailRead is how much is read to find them: a crash loop prints the
// same line once per restart, and five copies of it say less than one.
const (
	serviceTailLines = 5
	serviceTailRead  = 50
)

// serviceTail is the last few distinct lines one service printed, joined into
// one line, or "" when they cannot be read.
func (e *Engine) serviceTail(ctx context.Context, up *composeLine, service string) string {
	out, err := e.cmd.Output(ctx, up.conn.env, up.conn.bin, "compose", "-p", up.name, "logs",
		"--no-color", "--no-log-prefix", "--tail", strconv.Itoa(serviceTailRead), service)
	if err != nil {
		return ""
	}
	var lines []string
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSpace(line)
		if line == "" || slices.Contains(lines, line) {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) > serviceTailLines {
		lines = lines[len(lines)-serviceTailLines:]
	}
	return strings.Join(lines, " | ")
}
