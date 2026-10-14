//go:build !windows

package proxy

import (
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/logger"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// The astro 1.x CLI runs its proxy as `astro dev proxy serve --port <port>`.
// No 2.x build has a dev tree, so these arguments name a 1.x proxy and nothing
// else: not this CLI's daemon (ServeSubcommand), and not Astro Desktop's
// proxy, which runs inside the desktop app.
var v1ServeArgs = []string{"dev", "proxy", "serve"}

// An astro proxy, 1.x or 2.x, answers a hostname it has no route for with a
// page carrying notFoundHeading. The takeover asks for that page with
// takeoverProbeHost, which no route can use, and not for the landing page,
// because a 1.x landing page prunes routes.json and writes it back.
const (
	notFoundHeading   = "<h1>Project Not Found</h1>"
	takeoverProbeHost = "astro-proxy-takeover.invalid"
)

// process is what the takeover needs to know about a running process.
type process struct {
	pid  int
	args []string
	env  []string
}

const (
	// stopTimeout is how long a 1.x proxy gets to exit on SIGTERM before it
	// is killed. It counts against the routes lock pkg/proxy's EnsureRunning
	// holds while BeforeStart runs, which is sized for it.
	stopTimeout  = 5 * time.Second
	stopPollWait = 500 * time.Millisecond

	probeTimeout   = 500 * time.Millisecond
	probeBodyLimit = 64 << 10
)

// beforeStart is what the daemon runs before every start.
var beforeStart = takeOverFromV1

// listOwnProcesses returns this user's processes whose arguments and
// environment can be read. It is a seam so tests never see real processes.
var listOwnProcesses = ownProcesses

// stopProcess ends pid: SIGTERM, then SIGKILL once stopTimeout has passed.
var stopProcess = func(pid int) {
	syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck // waitForExit checks the outcome
	waitForExit(pid)
}

// waitForExit polls until pid is gone, escalating to SIGKILL once stopTimeout
// has passed.
func waitForExit(pid int) {
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(stopPollWait)
		if !pkgproxy.IsPIDAlive(pid) {
			return
		}
	}
	syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // nothing left to try
	time.Sleep(stopPollWait)
}

// takeOverFromV1 stops the astro 1.x proxy holding port, so this CLI's daemon
// can serve there instead of on a port that changes on every start.
//
// It stops a process only when all of these hold:
//   - something is listening on port;
//   - exactly one of this user's processes runs `dev proxy serve` for port;
//   - that process reads the same route store as this CLI, so its projects
//     keep their URLs when this CLI's daemon serves the store instead;
//   - port answers an unknown hostname with the astro proxy's not-found page.
//
// A 1.x proxy exits when it cannot bind its port, so a live one serving port
// is the process that holds it. Anything else on port stays, and the daemon
// falls back to another port.
//
// The probe runs first, so the processes are listed only when an astro proxy
// holds port, and the signal follows the listing without a wait between them.
func takeOverFromV1(port string) {
	if pkgproxy.IsPortAvailable(port) || !answersAsAstroProxy(port) {
		return
	}
	store := sameDir(Routes().Dir())
	var holders []int
	for _, p := range listOwnProcesses() {
		if v1ProxyPort(p.args) == port && sameDir(v1RouteStore(p.env)) == store {
			holders = append(holders, p.pid)
		}
	}
	if len(holders) != 1 {
		return
	}
	logger.Debugf("stopping the astro 1.x proxy on port %s (PID %d); this CLI's proxy serves its projects from the same routes", port, holders[0])
	stopProcess(holders[0])
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
		return pkgproxy.DefaultPort
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
	return filepath.Join(home, config.ConfigDir, proxyDir)
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

// probeProxy GETs / on port with the given Host, within probeTimeout, and
// returns the response with the start of its body.
func probeProxy(port, host string) (resp *http.Response, body []byte, ok bool) {
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+"/", http.NoBody)
	if err != nil {
		return nil, nil, false
	}
	req.Host = host

	client := &http.Client{Timeout: probeTimeout}
	resp, err = client.Do(req)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(io.LimitReader(resp.Body, probeBodyLimit))
	return resp, body, err == nil
}
