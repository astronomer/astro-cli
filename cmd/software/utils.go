package software

import (
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func VersionMatchCmds(rootCmd *cobra.Command, parent []string) {
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		cmdName := strings.Join(parent, " ")
		flagRestriction, ok := FlagAvailabilityByVersion[cmdFlag{cmdName, f.Name}]
		if ok && !houston.VerifyVersionMatch(houstonVersion, flagRestriction) {
			f.Hidden = true
		}
	})
	for _, cm := range rootCmd.Commands() {
		cmdName := fmt.Sprintf("%s %s", strings.Join(parent, " "), cm.Name())
		cmdRestriction, ok := CmdAvailabilityByVersion[cmdName]
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
		fmt.Printf("Error: unknown command \"%s\" for \"astro\" \nRun 'astro --help' for usage.\n", c.Name())
	}
	c.ResetCommands()           // remove all the subcommands
	c.DisableFlagParsing = true // to disable help flag
}

var CmdAvailabilityByVersion = map[string]houston.VersionRestrictions{
	"astro team":            {GTE: "0.31.0"},
	"astro deployment team": {GTE: "0.30.0"},
	"astro workspace team":  {GTE: "0.30.0"},

	"astro deployment runtime": {GTE: "0.29.0"},
}

type cmdFlag struct {
	cmd  string
	flag string
}

var FlagAvailabilityByVersion = map[cmdFlag]houston.VersionRestrictions{}
