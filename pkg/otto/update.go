package otto

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/astronomer/astro-cli/config"
)

const (
	updateCheckInterval = 24 * time.Hour
	updateStateFile     = ".update-check"
)

// updateState is the cached result of the last CDN version check. Persisted
// next to the Otto binary so the hint can be printed synchronously on startup
// without a network call on the hot path.
type updateState struct {
	LastCheck   string `json:"lastCheck"`
	LatestKnown string `json:"latestKnown"`
}

// hintUpdateAvailable writes a one-line upgrade notice to w if the cached
// latest version is newer than the installed one. Synchronous, zero network —
// callers should invoke it before redirecting stderr/stdout so the notice
// reaches the user.
func hintUpdateAvailable(w io.Writer) {
	state, err := readUpdateState()
	if err != nil || state.LatestKnown == "" {
		return
	}
	installed, err := InstalledVersion()
	if err != nil || installed == "" {
		return
	}
	if isVersionNewer(state.LatestKnown, installed) {
		fmt.Fprintf(w, "Otto %s is available. Run `astro otto update` to upgrade.\n", state.LatestKnown)
	}
}

// autoUpdate downloads and installs the cached-latest Otto when it's newer
// than the installed version. Synchronous — runs before the TUI takes over
// so the progress line reaches the user. Soft-fails: any error (network,
// disk) prints a notice and returns, letting the caller continue with the
// existing binary.
//
// `download` is parameterized so tests can swap in a stub — production
// callers pass downloadAndInstall. Gating on the `otto.auto_update` flag
// is the caller's responsibility; this function always applies when called.
func autoUpdate(w io.Writer, download func() error) {
	installed, err := InstalledVersion()
	if err != nil || installed == "" {
		return // first-run case — EnsureBinary already fetched latest
	}
	state, err := readUpdateState()
	if err != nil || state.LatestKnown == "" {
		return // no cached version yet; next launch will know more
	}
	if !isVersionNewer(state.LatestKnown, installed) {
		return
	}
	fmt.Fprintf(w, "Updating Otto %s → %s...\n", installed, state.LatestKnown)
	if err := download(); err != nil {
		fmt.Fprintf(w, "Otto update failed (%v); continuing with %s\n", err, installed)
	}
}

// autoUpdateEnabled reads the `otto.auto_update` flag. Default is true via
// the newCfg("otto.auto_update", "true") registration in config/config.go.
func autoUpdateEnabled() bool {
	return config.CFG.OttoAutoUpdate.GetBool()
}

// refreshUpdateCacheAsync refreshes the cached latest version if the cache is
// older than updateCheckInterval. Safe to run in a goroutine after the logger
// is redirected — it never prints, only updates the state file.
func refreshUpdateCacheAsync() {
	state, _ := readUpdateState()
	if t, err := time.Parse(time.RFC3339, state.LastCheck); err == nil {
		if time.Since(t) < updateCheckInterval {
			return
		}
	}
	latest, err := LatestVersion()
	if err != nil {
		return
	}
	_ = writeUpdateState(updateState{
		LastCheck:   time.Now().UTC().Format(time.RFC3339),
		LatestKnown: latest,
	})
}

func readUpdateState() (updateState, error) {
	var state updateState
	data, err := os.ReadFile(filepath.Join(BinDir(), updateStateFile))
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func writeUpdateState(state updateState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(BinDir(), dirPerm); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(BinDir(), updateStateFile), data, stateFilePerm)
}

// isVersionNewer reports whether latest is a strictly greater semver than
// installed. Unparseable installed → treat as "upgrade suggested"; unparseable
// latest → treat as "no upgrade" (we can't assert newer without a valid rhs).
func isVersionNewer(latest, installed string) bool {
	iv, err := semver.NewVersion(installed)
	if err != nil {
		return true
	}
	lv, err := semver.NewVersion(latest)
	if err != nil {
		return false
	}
	return lv.GreaterThan(iv)
}
