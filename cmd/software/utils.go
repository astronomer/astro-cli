package software

import (
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/spf13/cobra"
)

var cmdAvailabilityByVersion = map[string]houston.VersionRestrictions{
	"astro team update": {GTE: "0.29.2"},

	"astro deployment runtime": {GTE: "0.29.0"},

	"astro deployment team": {GTE: "0.28.0"},
	"astro workspace team":  {GTE: "0.28.0"},
	"astro team":            {GTE: "0.28.0"},
}

func VersionMatchCmds(rootCmd *cobra.Command, parent []string) {
	for _, cm := range rootCmd.Commands() {
		cmdName := fmt.Sprintf("%s %s", strings.Join(parent, " "), cm.Name())
		cmdRestriction, ok := cmdAvailabilityByVersion[cmdName]
		if ok && !houston.VerifyVersionMatch(houstonVersion, cmdRestriction) {
			removeCmd(cm)
			continue // no need to check subcommands as that has been removed by removeCmd
		}
		VersionMatchCmds(cm, append(parent, cm.Name()))
	}
}

func removeCmd(c *cobra.Command) {
	c.Hidden = true                                   // hide the command in help output
	c.Run = func(cmd *cobra.Command, args []string) { // define the error response when the command is executed
		fmt.Printf("Error: unknown command \"%s\" for \"astro\" \nRun 'astro --help' for usage.\n\nAstro Private Cloud Version: %s\nMake sure you are using right set of commands for the connected platform version\n\n", c.Name(), houstonVersion)
	}
	c.ResetCommands()           // remove all the subcommands
	c.DisableFlagParsing = true // to disable help flag
}
