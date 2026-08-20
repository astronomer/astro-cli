// Package localshared holds the pieces both local engines (standalone and
// docker) need: port choice, hostname fallback, proxy-route teardown, and daemon
// lifecycle. an earlier fix rule is that anything both modes need lives here, never
// copied — one copy is how "works in docker, broken in standalone" stops shipping.
// Like the engines it serves, it follows the layer rules in
// docs/v2-architecture.md: nothing here prints.
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

	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
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

// PlanHostname uses the plan's hostname, deriving one only when plan
// building has not (the same derivation internal/project uses).
func PlanHostname(p rt.Plan, projectPath string) (string, error) {
	if p.Hostname != "" {
		return p.Hostname, nil
	}
	hostname, _, err := proxy.DeriveHostname(projectPath)
	return hostname, err
}

// RemoveRoute deregisters a project's proxy route; an empty hostname means
// no route was ever registered.
func RemoveRoute(s *proxy.Store, hostname string) error {
	if hostname == "" {
		return nil
	}
	if _, err := s.RemoveRoute(hostname); err != nil {
		return fmt.Errorf("removing proxy route %s: %w", hostname, err)
	}
	return nil
}
