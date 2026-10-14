package astro

import (
	"strings"

	"github.com/spf13/cobra"
)

// What the astro dbt group's commands say now. deploy and delete moved to
// astro deploy bundle, which deploys a dbt project, or any other non-DAG
// bundle, from its own directory. cleanup removed the Cosmos Boost sidecars
// earlier deploys wrote into the project; v2 writes none there, and leaves
// out the old ones from what it ships.
const (
	errDbtRemoved = "astro dbt was removed in Astro CLI v2: use astro deploy bundle, which deploys a dbt project " +
		"(or any other non-DAG bundle) from its directory, and astro deploy bundle --delete, which removes one"
	errDbtDeployRemoved = "astro dbt deploy was removed in Astro CLI v2: use astro deploy bundle [PATH] --deployment <Deployment id, name or link name>. " +
		"PATH replaces --project-path, --deployment replaces the argument and --deployment-name, and --workspace replaces --workspace-id; " +
		"--mount-path, --description, --wait and --wait-time are unchanged"
	errDbtDeleteRemoved = "astro dbt delete was removed in Astro CLI v2: use astro deploy bundle --delete [PATH] --deployment <Deployment id, name or link name>. " +
		"PATH replaces --project-path, --deployment replaces the argument and --deployment-name, and --workspace replaces --workspace-id; " +
		"--mount-path, --description, --wait and --wait-time are unchanged"
	errDbtCleanupRemoved = "astro dbt cleanup was removed in Astro CLI v2, with no replacement: " +
		"Astro CLI v2 no longer writes Cosmos Boost files into the project, and leaves out the ones earlier versions wrote " +
		"from every bundle it deploys and every image it generates. " +
		"Delete any old .astro/dbt_metadata.json and .astro/manifest.slim.json files by hand: " +
		"a local Airflow running the Cosmos Boost plugin, or the build of a declared Dockerfile, may still read them"
)

// newDbtRemovedCmd builds the astro dbt tombstone (cliout.RemovedCommand):
// one hidden stub that takes any subcommand, so astro dbt deploy and its
// 1.x flags reach the guidance for what was typed rather than dying on a
// flag.
func newDbtRemovedCmd() *cobra.Command {
	return removedCmdStub("dbt", nil, "Removed in v2 — use astro deploy bundle", dbtRemovedGuidance)
}

// dbtRemovedGuidance names the replacement for the subcommand typed, the
// first argument that is not a flag.
func dbtRemovedGuidance(args []string) string {
	if msg, ok := dbtSubcommandGuidance[dbtTypedSubcommand(args)]; ok {
		return msg
	}
	return errDbtRemoved
}

// dbtSubcommandGuidance is what each 1.x astro dbt subcommand says now.
var dbtSubcommandGuidance = map[string]string{
	"deploy":  errDbtDeployRemoved,
	"delete":  errDbtDeleteRemoved,
	"cleanup": errDbtCleanupRemoved,
}

// dbtValueFlags are the flags that took a value on 1.x's astro dbt commands
// (v1_flags.tsv), and the --output and --verbosity v2 reads anywhere, so a
// flag's value typed before the subcommand is not read as the subcommand.
var dbtValueFlags = map[string]bool{
	"deployment-name": true, "n": true,
	"description": true,
	"mount-path":  true, "m": true,
	"project-path": true, "p": true,
	"wait-time": true, "t": true,
	"workspace-id": true,
	"output":       true, "o": true,
	"verbosity": true,
}

// dbtTypedSubcommand is the first argument that is neither a flag nor a
// value one took: astro dbt -n prod delete names delete. It stops at "--".
func dbtTypedSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return ""
		case strings.HasPrefix(a, "--"):
			if name := a[2:]; !strings.Contains(name, "=") && dbtValueFlags[name] {
				i++
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// A group of shorthands: the first that takes a value takes the
			// rest of the group (-nprod), or the next word when it is the
			// group's last letter (-n prod, -wt 5m).
			for j := 1; j < len(a); j++ {
				if dbtValueFlags[a[j:j+1]] {
					if j == len(a)-1 {
						i++
					}
					break
				}
			}
		default:
			return a
		}
	}
	return ""
}
