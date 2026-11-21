package version

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	semver "github.com/Masterminds/semver/v3"

	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// noticeInterval is how long after the upgrade notice was printed it stays
// quiet. A person who has read it once does not need it on every command.
const noticeInterval = 24 * time.Hour

const upgradeURL = "https://www.astronomer.io/docs/astro/cli/upgrade-cli"

// noticeDue reports whether the notice has not been printed in the last day,
// by the stamp kept beside the release check in cachePath.
func noticeDue(cachePath string, now time.Time) bool {
	c, ok := readCheckFile(cachePath)
	if !ok || c.NoticeShownAt.IsZero() {
		return true
	}
	// A stamp in the future is stale too, or a clock set back would silence
	// the notice for good.
	age := now.Sub(c.NoticeShownAt)
	return age < 0 || age >= noticeInterval
}

// recordNoticeShown stamps now into cachePath, leaving the rest of the cached
// answer as it was.
func recordNoticeShown(cachePath string, now time.Time) {
	c, _ := readCheckFile(cachePath)
	c.NoticeShownAt = now
	writeCheckFile(cachePath, c)
}

// UpdateCheck is a release lookup running beside a command, so that the
// command never waits for it.
type UpdateCheck struct {
	cachePath string
	result    chan *semver.Version
	// grace is how long Print waits for a lookup still in flight; zero for an
	// answer that is already in hand.
	grace time.Duration
}

// lookupGrace bounds the wait for a lookup that a stale cache sent to the
// network. A run of fast commands would otherwise kill the lookup at exit every
// time, so the cache would never refresh and each run would waste a request.
const lookupGrace = 300 * time.Millisecond

// StartUpdateCheck looks for a newer release in the background and returns at
// once. It returns nil, and asks nothing, when the notice was printed within
// the last day. Collect the answer with Print after the command has finished.
func StartUpdateCheck(ctx context.Context, client *http.Client, cachePath string, now time.Time) *UpdateCheck {
	if !noticeDue(cachePath, now) {
		return nil
	}
	c := &UpdateCheck{cachePath: cachePath, result: make(chan *semver.Version, 1)}
	// A fresh cached answer needs no network: it is in hand before the command
	// starts, so even the fastest command shows the notice. Only a day whose
	// cache is stale races the lookup against the command.
	if latest, ok := cachedNewerRelease(cachePath, now); ok {
		c.result <- latest
		return c
	}
	go func() {
		// A lookup that fails says nothing: the notice is a courtesy.
		latest, err := NewerRelease(ctx, client, cachePath)
		if err != nil {
			logger.Debugf("Error comparing CLI versions: %v", err)
			latest = nil
		}
		c.result <- latest
	}()
	c.grace = lookupGrace
	return c
}

// Print writes the upgrade notice to w when the lookup has already found a
// newer release, and records that it did. A lookup still running is waited for
// at most lookupGrace and then skipped, so a command's exit is never held up
// by the network for long. Safe on a nil check.
func (c *UpdateCheck) Print(w io.Writer, now time.Time) {
	if c == nil {
		return
	}
	var latest *semver.Version
	select {
	case latest = <-c.result:
	default:
		if c.grace <= 0 {
			return
		}
		timer := time.NewTimer(c.grace)
		defer timer.Stop()
		select {
		case latest = <-c.result:
		case <-timer.C:
			return
		}
	}
	if latest == nil {
		return
	}
	fmt.Fprintf(w, "\nAstro CLI %s is available. You have %s.\n", latest, Current())
	fmt.Fprintf(w, "Upgrade: %s\n", ansi.ForStderr().Cyan(upgradeURL))
	fmt.Fprintln(w, "Turn this off: astro config set --global upgrade_message false")
	recordNoticeShown(c.cachePath, now)
}
