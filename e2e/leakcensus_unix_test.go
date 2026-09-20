//go:build e2e && !windows

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// daemons lists proxy daemons still serving from this run's binary.
//
// This axis needs no before-snapshot. The harness builds into a temp directory
// made minutes ago, so anything serving proxyServeArg from that path was started
// by this run and not reaped.
//
// Split from the rest of the census by platform because pgrep is: every other
// file in this package that reaches for it is !windows too, and a census that
// reported "pgrep: executable file not found" as a finding would fail the
// Windows tier-0 job on every run.
func daemons(ctx context.Context) (found []string, err error) {
	if astroBin == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, censusTimeout)
	defer cancel()
	// Quoted and bounded: pgrep -f takes an extended regular expression, not a
	// literal, so a temp path carrying a "." would match more than itself, and
	// one carrying a bracket would fail to compile and exit 2.
	pattern := regexp.QuoteMeta(astroBin + " " + proxyServeArg)
	out, cmdErr := exec.CommandContext(ctx, "pgrep", "-f", pattern).Output()
	if cmdErr != nil {
		var exit *exec.ExitError
		if errors.As(cmdErr, &exit) && exit.ExitCode() == 1 {
			// pgrep exits 1 when nothing matched, which is the answer here
			// rather than a failure.
			return nil, nil
		}
		// Anything else is reported rather than read as an empty answer: a
		// pgrep that cannot run would otherwise report zero daemons forever.
		return nil, fmt.Errorf("listing proxy daemons: %w", cmdErr)
	}
	for _, pid := range strings.Fields(string(out)) {
		found = append(found, "proxy daemon pid "+pid)
	}
	return found, nil
}
