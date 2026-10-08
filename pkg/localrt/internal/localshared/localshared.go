// Package localshared holds the pieces both local engines (standalone and
// docker) need: port choice, hostname fallback, proxy-route teardown, and daemon
// lifecycle. The rule is that anything both modes need lives here, never
// copied — one copy is how "works in docker, broken in standalone" stops shipping.
// Like the engines it serves, it follows the layer rules in
// docs/architecture.md: nothing here prints.
//
// State callbacks and line splitting used to live here too. They moved up to the
// contract package (rt.OnState, rt.LineWriter) because they exist to feed
// rt.Callbacks, and because pkg/imagebuild — a separate module — needs the line
// writer and cannot reach an engine-private one.
package localshared

import (
	"fmt"
	"strconv"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// EnsureDaemon brings the proxy daemon up after a route is registered. It is
// best-effort: a failure is reported through the callback and never fails the
// start, since Airflow is still reachable on its direct localhost port. A nil
// daemon is a no-op.
func EnsureDaemon(d rt.ProxyDaemon, cb rt.Callbacks, now time.Time, hostname string) {
	if d == nil {
		return
	}
	if _, err := d.EnsureRunning(); err != nil && cb.OnLine != nil {
		cb.OnLine(rt.LogLine{
			Component: "system",
			Time:      now,
			Text:      fmt.Sprintf("could not start the local proxy for %s: %s; reach Airflow on its localhost port", hostname, err),
		})
	}
}

// ReapDaemon stops the proxy daemon when no routes remain. Best-effort, and a
// no-op for a nil daemon.
func ReapDaemon(d rt.ProxyDaemon) {
	if d != nil {
		d.StopIfEmpty()
	}
}

// allocAttempts bounds the retries when the pool hands back a port already
// claimed by a sibling allocation in the same Start. The pool is thousands of
// ports wide, so a clash is rare and clears on the next draw; the cap only
// guards against a degenerate allocator looping forever.
const allocAttempts = 50

// ChoosePort resolves one published port: the requested one when free, the
// mode default when free, otherwise an allocated one from the proxy pool.
// portFree and alloc are the engine's seams so tests never bind sockets.
//
// exclude names ports already chosen for this same project this Start but not
// yet written to routes.json — the sibling port a two-port mode (docker's api
// and postgres) picked a moment earlier. Without it both draws can land on the
// same pool port, since neither sees the other until the routes are saved.
func ChoosePort(requested, fallback int, portFree func(port string) bool, alloc func() (string, error), exclude ...int) (int, error) {
	blocked := func(p int) bool {
		for _, e := range exclude {
			if p == e {
				return true
			}
		}
		return false
	}
	for _, p := range []int{requested, fallback} {
		if p > 0 && !blocked(p) && portFree(strconv.Itoa(p)) {
			return p, nil
		}
		if p == requested && p > 0 {
			// A busy requested port falls through to allocation, not to
			// the default: the caller asked for that port specifically,
			// and the default may collide with another project.
			break
		}
	}
	for range allocAttempts {
		s, err := alloc()
		if err != nil {
			return 0, err
		}
		p, err := strconv.Atoi(s)
		if err != nil {
			return 0, err
		}
		if !blocked(p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("could not allocate a port distinct from %v", exclude)
}

// PlanHostname returns the hostname a starting project asks for: the plan's,
// derived only when plan building has not done it (the same derivation
// internal/project uses).
//
// Asks for, not gets. The derived name is the directory's base name, so two
// projects in directories called the same thing — ~/work/analytics and
// ~/personal/analytics — ask for the same one. Store.AddRoute settles it,
// under the routes lock, and writes back what the project actually got; the
// engines record that rather than this.
func PlanHostname(p rt.Plan, projectPath string) (string, error) {
	if p.Hostname != "" {
		return p.Hostname, nil
	}
	// From the canonical path, for the reason composeProjectName gives: the
	// label is a sanitized directory name, and the two Unicode spellings of
	// one name sanitize differently, so one directory would ask for two
	// hostnames and the second would be told its own name was taken.
	if canonical, cErr := rt.CanonicalPath(projectPath); cErr == nil {
		projectPath = canonical
	}
	hostname, _, err := proxy.DeriveHostname(projectPath)
	return hostname, err
}

// HostnameDiscriminator is what AddRoute folds into a hostname when another
// project already holds it: the leading characters of the project's path
// hash, the same identity everything else here is keyed on.
//
// Empty when the path has no id — a broken symlink in it, say. AddRoute reads
// that as "no way to tell these two apart" and refuses the duplicate, which
// is what it did before any of this existed. A display name is never worth
// failing a start over.
func HostnameDiscriminator(projectPath string) string {
	id, err := rt.ProjectID(projectPath)
	if err != nil {
		return ""
	}
	return id[:proxy.HostnameIDLen]
}

// RemoveRoute deregisters a project's proxy route and, when that was the last
// one, puts the proxy away. An empty hostname means no route was ever
// registered, so there is nothing to remove and nothing to conclude.
//
// The reap lives here, at the one place a localrt route is ever removed,
// rather than at each caller. Three call this — both engines' Stop and the
// --clean sweep — and a fourth arrives whenever somebody adds a teardown, so
// the alternative is a list of sites that has to be re-audited by hand and is
// wrong the first time it is not.
//
// It reaps on the count this removal returned rather than by asking the daemon
// to decide. Asking meant airflow/proxy.StopIfEmpty, which re-lists through a
// store built with no liveness predicate — the default one, which evicts a
// route whose recorded PID is dead. That is not the same question: a
// standalone master exits before the children in its group during shutdown, so
// the default predicate calls a route dead while localprune, reading the
// record's process group, correctly calls it alive. ListRoutes writes back
// what it pruned, so consulting that store did not merely answer wrongly, it
// deleted a live project's route on the way. The count here comes from this
// store, which carries the record-aware predicate, and costs no second pass.
func RemoveRoute(s *proxy.Store, hostname string, d rt.ProxyDaemon) error {
	if hostname == "" {
		return nil
	}
	remaining, err := s.RemoveRoute(hostname)
	if err != nil {
		return fmt.Errorf("removing proxy route %s: %w", hostname, err)
	}
	if remaining == 0 {
		ReapDaemon(d)
	}
	return nil
}
