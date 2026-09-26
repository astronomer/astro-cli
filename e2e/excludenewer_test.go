//go:build e2e

package e2e

import (
	"os"
	"strings"
)

// pinnedExcludeNewer is the date every uv in the suite resolves as of: only
// distributions uploaded before it exist, so a case builds the same
// environment today as on the day the constant was set.
//
// Without it, tier 1 resolves against live PyPI, and an upstream release turns
// every pull request red at once for a reason none of them caused — SQLAlchemy
// 2.1.0 did exactly that on 2026-09-24. With it, that kind of break lands on
// the unpinned nightly (nightly-e2e.yaml), which files an issue, and pull
// requests keep testing their own change.
//
// It reaches every uv the suite runs as UV_EXCLUDE_NEWER, set in project.env
// after the UV_ prefix is stripped: the harness's own `uv sync`, and each uv
// the CLI runs, because pkg/uv's childEnv passes the inherited environment
// through (the CLI never sets uv.Options.HermeticEnv, the one mode that would
// strip it, and --no-config covers files, not variables).
// TestTheCLIsUVHonorsTheResolutionDate proves the second half.
//
// # Bumping it
//
// .github/workflows/bump-e2e-exclude-newer.yml moves it to the current day
// weekly through scripts/bump-e2e-exclude-newer.sh and opens a PR; that PR's
// tier-1 run is the review. By hand:
//
//	scripts/bump-e2e-exclude-newer.sh            # today, 00:00 UTC
//	scripts/bump-e2e-exclude-newer.sh 2026-10-01 # a chosen day
//
// Move it when the bump PR is green, or when a pull request needs something
// published after it — most often a new Airflow series: when
// runtimeversions.FallbackAirflowSeries moves past what existed on this date,
// every tier-1 case fails with "No solution found" and a uv hint naming
// exclude-newer, and the fix is this constant rather than the scaffold. A red
// bump PR means upstream broke something since the last date, which the
// nightly will already have reported; hold the date until it is dealt with.
//
// The format is the one the bump script writes and matches: midnight UTC as
// an RFC 3339 timestamp.
const pinnedExcludeNewer = "2026-09-24T00:00:00Z"

// excludeNewerEnv overrides pinnedExcludeNewer for one run. Unset or empty
// takes the constant; "none" resolves against live PyPI, which is what the
// unpinned nightly sets; anything else is handed to uv as the cutoff.
//
// Its own variable rather than an ambient UV_EXCLUDE_NEWER, which the harness
// strips with the rest of UV_: a developer's personal uv setting is exactly the
// kind of leak that list exists to stop, and should not decide what the suite
// resolves.
const excludeNewerEnv = "ASTRO_E2E_EXCLUDE_NEWER"

// excludeNewer is the cutoff this run hands uv, or "" for none.
func excludeNewer() string {
	switch v := strings.TrimSpace(os.Getenv(excludeNewerEnv)); v {
	case "":
		return pinnedExcludeNewer
	case "none":
		return ""
	default:
		return v
	}
}
