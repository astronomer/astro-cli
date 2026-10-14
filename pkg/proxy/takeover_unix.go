//go:build !windows

package proxy

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// The astro 1.x CLI runs its proxy as `astro dev proxy serve --port <port>`.
// No 2.x build has a dev tree, so these arguments name a 1.x proxy and nothing
// else: not this daemon (ServeSubcommand), whichever tool started it.
var v1ServeArgs = []string{"dev", "proxy", "serve"}

// An astro proxy, 1.x or 2.x, answers a hostname it has no route for with a
// page carrying notFoundHeading. The takeover asks for that page with
// takeoverProbeHost, which no route can use, and not for the landing page,
// because a 1.x landing page prunes routes.json and writes it back.
const (
	notFoundHeading   = "<h1>Project Not Found</h1>"
	takeoverProbeHost = "astro-proxy-takeover.invalid"
)

// A 1.x CLI keeps its routes in <home>/.astro/proxy, where home is ASTRO_HOME
// or, unset, HOME.
const (
	v1ConfigDir = ".astro"
	v1ProxyDir  = "proxy"
)

// process is what the takeover needs to know about a running process.
type process struct {
	pid  int
	args []string
	env  []string
}

// listOwnProcesses returns this user's processes whose arguments and
// environment can be read. It is a seam so tests never see real processes.
var listOwnProcesses = ownProcesses

// stopV1Proxy ends pid: SIGTERM, then the forced kill once stopTimeout has
// passed. A seam so tests never signal a real process unless they mean to.
var stopV1Proxy = func(pid int) {
	syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck // waitForExit checks the outcome
	waitForExit(pid)
}

// takeOverFromV1 stops the astro 1.x proxy holding port, so the daemon can
// serve there instead of on a port that changes on every start.
//
// It is the daemon's own step rather than a host's, because which tool starts
// the daemon is an accident of which ran first: a host that skipped it would
// leave the daemon on a fallback port, and every tool after it would adopt that
// daemon there, so the 1.x proxy would keep 6563 for as long as it ran.
//
// It stops a process only when all of these hold:
//   - something is listening on port;
//   - port answers an unknown hostname with the astro proxy's not-found page;
//   - exactly one of this user's processes runs `dev proxy serve` for port;
//   - that process reads this daemon's own route store, so its projects keep
//     their URLs when this daemon serves the store instead.
//
// A 1.x proxy exits when it cannot bind its port, so a live one serving port
// is the process that holds it. Anything else on port stays, and the daemon
// falls back to another port.
//
// The probe runs first, so the processes are listed only when an astro proxy
// holds port, and the signal follows the listing without a wait between them.
func (d *Daemon) takeOverFromV1(port string) {
	if IsPortAvailable(port) || !answersAsAstroProxy(port) {
		return
	}
	store := sameDir(d.Store.Dir())
	var holders []int
	for _, p := range listOwnProcesses() {
		if v1ProxyPort(p.args) == port && sameDir(v1RouteStore(p.env)) == store {
			holders = append(holders, p.pid)
		}
	}
	if len(holders) != 1 {
		return
	}
	d.logf("stopping the astro 1.x proxy on port %s (PID %d); this daemon serves its projects from the same routes", port, holders[0])
	stopV1Proxy(holders[0])
}

// v1ProxyPort is the port a 1.x `dev proxy serve` command line serves, or ""
// when args are not one.
func v1ProxyPort(args []string) string {
	for i := 1; i+len(v1ServeArgs) <= len(args); i++ {
		if !slices.Equal(args[i:i+len(v1ServeArgs)], v1ServeArgs) {
			continue
		}
		flags := args[i+len(v1ServeArgs):]
		for j, f := range flags {
			if v, ok := strings.CutPrefix(f, "--port="); ok {
				return v
			}
			if f == "--port" && j+1 < len(flags) {
				return flags[j+1]
			}
		}
		return DefaultPort
	}
	return ""
}

// v1RouteStore is the route directory a 1.x CLI with environment env uses:
// $ASTRO_HOME/.astro/proxy, or $HOME/.astro/proxy when ASTRO_HOME is unset.
// A relative home gives "", since it is relative to a working directory this
// process does not know.
func v1RouteStore(env []string) string {
	home := envValue(env, "ASTRO_HOME")
	if home == "" {
		home = envValue(env, "HOME")
	}
	if !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Join(home, v1ConfigDir, v1ProxyDir)
}

func envValue(env []string, key string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}

// sameDir is dir in a form two spellings of one directory agree on.
func sameDir(dir string) string {
	if dir == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return filepath.Clean(dir)
}

// answersAsAstroProxy reports whether port answers an unknown hostname with
// an astro proxy's not-found page.
func answersAsAstroProxy(port string) bool {
	resp, body, ok := probeProxy(port, takeoverProbeHost)
	return ok && resp.StatusCode == http.StatusNotFound && strings.Contains(string(body), notFoundHeading)
}
