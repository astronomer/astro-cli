//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A core command must not leave config/ state on a machine that has none.
//
// main calls config.InitConfig before cobra has parsed argv, so the config/
// package initializes for every invocation. It used to create the home config
// as a side effect of reading it, which meant the core tree — which reads no config/
// setting at all — wrote ~/.astro/config.yaml and ~/.astro/config.yaml.lock
// into a fresh home.
//
// The read itself stays: cmd/root.go's detectRootOptions asks
// context.IsCloudContext() whether to mount the cloud or the software
// subtree, so the shape of the command tree depends on the config file before
// anyone knows which command was typed. What this asserts is that the read
// leaves nothing behind.
//
// The rule is about the core tree specifically, not about the binary. A shell
// command still writes: its PersistentPreRunE runs the telemetry hook, and
// the first-run notice records that it has been shown. Measured, `astro
// version` on a fresh home leaves a 1083-byte config.yaml for that reason —
// shell behavior doing what it means to do. Core commands carry the skip-pre-run
// annotation, so none of that runs for them, except the removal stubs, which
// are recorded only where the notice was already shown.
//
// Which is also the limit of what the first two cases prove. The harness sets
// ASTRO_TELEMETRY_DISABLED=1 for every run, so if a core command ever lost its
// annotation, the telemetry write it started doing would not show up in them.
// That half is checked structurally instead, by TestTreeInvariants in
// cmd/local. What these two own is the config read: that InitConfig, which
// runs for every invocation whatever the annotation says, creates nothing.
// The stubs' case turns telemetry on, pointed at an address nothing listens
// on (telemetryOn).
//
// Tier 0: the harness points HOME and ASTRO_HOME at the test's own temp
// directory, so anything found here was created by the run.
func TestInitLeavesNoHomeConfigBehind(t *testing.T) {
	tier(t, 0)
	p := newProject(t)

	p.run("init", "--name", "leaves-nothing").requireSuccess()

	checkNoHomeConfig(t, p)
	checkVaultUntouched(t, p)
}

// The same for a core command that fails, which is the case most likely to take
// a path nobody thought about: the `astro dev` stub refuses and exits
// non-zero, and it still must not write.
func TestAFailingCoreCommandLeavesNoHomeConfigBehind(t *testing.T) {
	tier(t, 0)
	p := newProject(t)

	p.run("dev", "ps", "--output", "json").requireFailure()

	checkNoHomeConfig(t, p)
	checkVaultUntouched(t, p)
}

// telemetryOn turns telemetry back on for one run, sending to an address
// nothing listens on, so no event reaches production analytics. The harness's
// ASTRO_TELEMETRY_DISABLED=1 is overridden, not removed: os/exec keeps the
// last of two values for one variable.
var telemetryOn = map[string]string{
	"ASTRO_TELEMETRY_DISABLED": "0",
	"ASTRO_TELEMETRY_API_URL":  "http://127.0.0.1:9/",
}

// The removal stubs `astro dev` and `astro run` are the core commands that do
// run part of the root's pre-run: it records them, so we learn when nobody
// types them any more. On a fresh machine that must still leave nothing
// behind, with telemetry on, which the cases above cannot see: the event is
// sent only where the first-run notice has already been shown, and the stub
// shows no notice of its own.
func TestARemovedCommandLeavesNoHomeConfigBehindWithTelemetryOn(t *testing.T) {
	tier(t, 0)

	// The control: with telemetry on, a shell command shows the notice and
	// records it, so the override above does turn telemetry on.
	control := newProject(t)
	r := control.runWith(telemetryOn, "version")
	r.requireSuccess()
	if !strings.Contains(r.Stderr, "collects usage data") {
		t.Fatalf("telemetry is not on for these runs: `astro version` showed no notice\nstderr:\n%s", r.Stderr)
	}
	if _, err := os.Stat(filepath.Join(control.home, ".astro", "config.yaml")); err != nil {
		t.Fatalf("telemetry is not on for these runs: `astro version` recorded no notice: %v", err)
	}

	p := newProject(t)
	for _, args := range [][]string{{"dev", "ps"}, {"run", "my_dag"}, {"dev", "start", "-o", "json"}} {
		r := p.runWith(telemetryOn, args...)
		r.requireFailure()
		if strings.Contains(r.Stderr, "collects usage data") {
			t.Errorf("astro %s showed the telemetry notice:\n%s", strings.Join(args, " "), r.Stderr)
		}
	}
	checkNoHomeConfig(t, p)
	checkVaultUntouched(t, p)
}

// checkNoHomeConfig fails if the run created a home config, its lock, or the
// directory they would sit in.
//
// The directory matters on its own: a regression that created ~/.astro and
// then bailed before writing — a saveConfig that took the lock and failed —
// would leave the file checks green.
func checkNoHomeConfig(t *testing.T, p *project) {
	t.Helper()
	dir := filepath.Join(p.home, ".astro")
	for _, name := range []string{"config.yaml", "config.yaml.lock"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("a core command created config/ state at %s (stat: %v)", path, err)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("a core command created %s (stat: %v)", dir, err)
	}
}
