package apc

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/houston"
)

var cmdAvailabilityByVersion = map[string]houston.VersionRestrictions{
	"astro team update": {GTE: "0.29.2"},

	"astro deployment runtime": {GTE: "0.29.0"},

	"astro deployment adopt":   {GTE: "2.1.0"},
	"astro deployment unadopt": {GTE: "2.1.0"},

	"astro deployment team": {GTE: "0.28.0"},
	"astro workspace team":  {GTE: "0.28.0"},
	"astro team":            {GTE: "0.28.0"},
}

func VersionMatchCmds(rootCmd *cobra.Command, parent []string) {
	for _, cm := range rootCmd.Commands() {
		cmdName := fmt.Sprintf("%s %s", strings.Join(parent, " "), cm.Name())
		cmdRestriction, ok := cmdAvailabilityByVersion[cmdName]
		if ok && !houston.VerifyVersionMatch(houstonVersion, cmdRestriction) {
			removeCmd(cm, cmdName, cmdRestriction)
			continue // no need to check subcommands as that has been removed by removeCmd
		}
		VersionMatchCmds(cm, append(parent, cm.Name()))
	}
}

// removeCmd takes a command this platform version cannot serve out of the help
// and makes running it fail.
//
// It does not delete the command, because the guidance is the point: someone who
// types a command their platform is too old for needs to be told which version
// adds it, not that the word does not exist. The error names the command, what
// it needs, and what is connected.
func removeCmd(c *cobra.Command, cmdName string, r houston.VersionRestrictions) {
	c.Hidden = true              // out of the help; the error below is the only way to meet it
	c.Args = cobra.ArbitraryArgs // clear any Args validator (e.g. cobra.ExactArgs) so a missing positional does not error before RunE below
	c.Run = nil                  // cobra prefers RunE over Run when both are set; this command has only the one below
	c.RunE = func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("`%s` needs Astro Private Cloud %s; this platform reports %s", cmdName, versionNeeded(r), houstonVersion)
	}
	c.SilenceUsage = true       // the version is the whole message; a usage block buries it
	c.ResetCommands()           // remove all the subcommands
	c.DisableFlagParsing = true // to disable help flag
}

// versionNeeded renders a restriction as the phrase that completes "needs Astro
// Private Cloud ...".
func versionNeeded(r houston.VersionRestrictions) string {
	if len(r.EQ) > 0 {
		return strings.Join(r.EQ, " or ")
	}
	switch {
	case r.GTE != "" && r.LT != "":
		return fmt.Sprintf("%s or newer, below %s", r.GTE, r.LT)
	case r.LT != "":
		return fmt.Sprintf("older than %s", r.LT)
	default:
		return r.GTE + " or newer"
	}
}
