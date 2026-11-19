package localshared

import (
	"context"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
)

// stderrTail is how much of a failed command's stderr a debug line keeps: the
// end, where a CLI says why it stopped.
const stderrTail = 2048

// CommandLog is the debug log of one external command an engine runs (docker,
// docker compose, podman, the detached airflow standalone). Nothing reaches
// the log below debug, the level astro local --verbosity debug turns on.
//
// The command line is logged as given; the environment the engine adds is
// logged by name only, because that is where a project's secrets travel.
type CommandLog struct {
	ctx   context.Context
	start time.Time
	attrs []any
}

// StartCommand logs that name is about to run with args and the extra
// environment env, and returns the log to End once it has.
func StartCommand(ctx context.Context, env []string, name string, args ...string) *CommandLog {
	c := &CommandLog{ctx: ctx, start: time.Now(), attrs: []any{"cmd", CommandLine(name, args)}}
	if names := EnvNames(env); names != "" {
		c.attrs = append(c.attrs, "env", names)
	}
	slog.DebugContext(ctx, "running", c.attrs...)
	return c
}

// Debug reports whether the command's lines are written, for a caller that
// would otherwise capture output only to log it.
func (c *CommandLog) Debug() bool {
	return slog.Default().Enabled(c.ctx, slog.LevelDebug)
}

// End logs how the command ended: how long it took and, when it failed, its
// error and the end of whatever stderr the caller captured for it.
func (c *CommandLog) End(err error, stderr []byte) {
	attrs := append(slices.Clip(c.attrs), "took", time.Since(c.start).Round(time.Millisecond))
	if err == nil {
		slog.DebugContext(c.ctx, "finished", attrs...)
		return
	}
	attrs = append(attrs, "err", err)
	if s := strings.TrimSpace(string(stderr)); s != "" {
		if len(s) > stderrTail {
			s = "…" + s[len(s)-stderrTail:]
		}
		attrs = append(attrs, "stderr", s)
	}
	slog.DebugContext(c.ctx, "failed", attrs...)
}

// urlUserinfo is the user:password@ of a URL, up to the last @ before the
// path, so a password holding a raw @ is masked whole. (A / is not allowed
// unencoded in userinfo; stopping at one keeps a git ref's @ in the path.)
var urlUserinfo = regexp.MustCompile(`(://)[^/\s]*@`)

// CommandLine is name and args as one line, to read rather than to paste:
// arguments are not quoted, and credentials in a URL are masked.
func CommandLine(name string, args []string) string {
	return urlUserinfo.ReplaceAllString(strings.Join(append([]string{name}, args...), " "), "${1}***@")
}

// EnvNames is the names in a KEY=value environment, comma-separated, without
// their values.
func EnvNames(env []string) string {
	names := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		names = append(names, k)
	}
	return strings.Join(names, ",")
}
