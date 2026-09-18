//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// A v2 command must not leave v1 state on a machine that has none.
//
// main calls config.InitConfig before cobra has parsed argv, so the v1 config
// package initializes for every invocation. It used to create the home config
// as a side effect of reading it, which meant the v2 tree — which reads no v1
// setting at all — wrote ~/.astro/config.yaml and ~/.astro/config.yaml.lock
// into a fresh home.
//
// The read itself stays: cmd/root.go's detectRootOptions asks
// context.IsCloudContext() whether to mount the cloud or the software
// subtree, so the shape of the command tree depends on the config file before
// anyone knows which command was typed. What this asserts is that the read
// leaves nothing behind.
//
// The rule is about the v2 tree specifically, not about the binary. A v1
// command still writes: its PersistentPreRunE runs the telemetry hook, and
// the first-run notice records that it has been shown. Measured, `astro
// version` on a fresh home leaves a 1083-byte config.yaml for that reason —
// v1 behavior doing what it means to do. v2 commands carry the skip-pre-run
// annotation, so none of that runs for them.
//
// Which is also the limit of what these two cases prove. The harness sets
// ASTRO_TELEMETRY_DISABLED=1 for every run and cannot do otherwise, since
// there is no way to point the sender somewhere other than production — so
// if a v2 command ever lost its annotation, the telemetry write it started
// doing would not show up here. That half is checked structurally instead,
// by TestTreeInvariants in cmd/local. What these two own is the config read:
// that InitConfig, which runs for every invocation whatever the annotation
// says, creates nothing.
//
// Tier 0: the harness points HOME and ASTRO_HOME at the test's own temp
// directory, so anything found here was created by the run.
func TestInitLeavesNoV1ConfigBehind(t *testing.T) {
	tier(t, 0)
	p := newProject(t)

	p.run("init", "--name", "leaves-nothing").requireSuccess()

	checkNoV1Config(t, p)
	checkVaultUntouched(t, p)
}

// The same for a v2 command that fails, which is the case most likely to take
// a path nobody thought about: the `astro dev` stub refuses and exits
// non-zero, and it still must not write.
func TestAFailingV2CommandLeavesNoV1ConfigBehind(t *testing.T) {
	tier(t, 0)
	p := newProject(t)

	p.run("dev", "ps", "--output", "json").requireFailure()

	checkNoV1Config(t, p)
	checkVaultUntouched(t, p)
}

// checkNoV1Config fails if the run created a v1 home config, its lock, or the
// directory they would sit in.
//
// The directory matters on its own: a regression that created ~/.astro and
// then bailed before writing — a saveConfig that took the lock and failed —
// would leave the file checks green.
func checkNoV1Config(t *testing.T, p *project) {
	t.Helper()
	dir := filepath.Join(p.home, ".astro")
	for _, name := range []string{"config.yaml", "config.yaml.lock"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("a v2 command created v1 state at %s (stat: %v)", path, err)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("a v2 command created %s (stat: %v)", dir, err)
	}
}
