// Package localshared holds the pieces both local engines (standalone and
// docker) need: port choice, hostname fallback, proxy-route teardown, state
// callbacks, and line splitting. an earlier fix rule is that anything both modes
// need lives here, never copied — one copy is how "works in docker, broken
// in standalone" stops shipping. Like the engines it serves, it follows the
// layer rules in docs/v2-architecture.md: nothing here prints.
package localshared

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// ProxyDaemon is the local reverse-proxy daemon lifecycle the engines drive:
// start it once a route is registered so <name>.localhost resolves, reap it
// once the last route is gone. The composition root (cmd/local) supplies the
// airflow/proxy implementation; the engines never import that package, which
// pulls in config the v2 tree must not. A nil ProxyDaemon means no daemon —
// Windows, where the proxy is unsupported, and unit tests.
type ProxyDaemon interface {
	// EnsureRunning starts the daemon if it isn't already running and returns
	// the port it bound.
	EnsureRunning() (string, error)
	// StopIfEmpty stops the daemon when no routes remain, so the last project
	// to stop leaves no orphan behind.
	StopIfEmpty()
}

// EnsureDaemon brings the proxy daemon up after a route is registered. It is
// best-effort: a failure is reported through the callback and never fails the
// start, since Airflow is still reachable on its direct localhost port. A nil
// daemon is a no-op.
func EnsureDaemon(d ProxyDaemon, cb localrt.Callbacks, now time.Time, hostname string) {
	if d == nil {
		return
	}
	if _, err := d.EnsureRunning(); err != nil && cb.OnLine != nil {
		cb.OnLine(localrt.LogLine{
			Component: "system",
			Time:      now,
			Text:      fmt.Sprintf("could not start the local proxy for %s: %s; reach Airflow on its localhost port", hostname, err),
		})
	}
}

// ReapDaemon stops the proxy daemon when no routes remain. Best-effort, and a
// no-op for a nil daemon.
func ReapDaemon(d ProxyDaemon) {
	if d != nil {
		d.StopIfEmpty()
	}
}

// ChoosePort resolves one published port: the requested one when free, the
// mode default when free, otherwise an allocated one from the proxy pool.
// portFree and alloc are the engine's seams so tests never bind sockets.
func ChoosePort(requested, fallback int, portFree func(port string) bool, alloc func() (string, error)) (int, error) {
	for _, p := range []int{requested, fallback} {
		if p > 0 && portFree(strconv.Itoa(p)) {
			return p, nil
		}
		if p == requested && p > 0 {
			// A busy requested port falls through to allocation, not to
			// the default: the caller asked for that port specifically,
			// and the default may collide with another project.
			break
		}
	}
	s, err := alloc()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(s)
}

// PlanHostname uses the plan's hostname, deriving one only when plan
// building has not (the same derivation internal/project uses).
func PlanHostname(p localrt.Plan, projectPath string) (string, error) {
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

// OnState reports a state transition through the callback when one is set.
func OnState(cb localrt.Callbacks, s localrt.State, err error) {
	if cb.OnState != nil {
		cb.OnState(s, err)
	}
}

// LineWriter is an io.Writer that splits its input into lines and hands
// each complete one to Emit. Subprocesses write whole lines, but nothing
// guarantees write boundaries, so partial lines are buffered.
type LineWriter struct {
	buf  bytes.Buffer
	Emit func(string)
}

func (w *LineWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Partial line: put it back and wait for the rest.
			w.buf.WriteString(line)
			break
		}
		if line = strings.TrimRight(line, "\r\n"); line != "" {
			w.Emit(line)
		}
	}
	return len(p), nil
}

// Flush emits any trailing line that arrived without a newline.
func (w *LineWriter) Flush() {
	if rest := strings.TrimRight(w.buf.String(), "\r\n"); rest != "" {
		w.Emit(rest)
	}
	w.buf.Reset()
}
