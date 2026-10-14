package astro

import (
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
	errDbtDeployRemoved = "astro dbt deploy was removed in Astro CLI v2: use astro deploy bundle [PATH] --deployment <id or link name>. " +
		"PATH replaces --project-path, --deployment replaces the argument and --deployment-name, and --workspace replaces --workspace-id; " +
		"--mount-path, --description, --wait and --wait-time are unchanged"
	errDbtDeleteRemoved = "astro dbt delete was removed in Astro CLI v2: use astro deploy bundle --delete [PATH] --deployment <id or link name>. " +
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
	if len(args) > 0 {
		if msg, ok := dbtSubcommandGuidance[args[0]]; ok {
			return msg
		}
	}
	return errDbtRemoved
}

// dbtSubcommandGuidance is what each 1.x astro dbt subcommand says now.
var dbtSubcommandGuidance = map[string]string{
	"deploy":  errDbtDeployRemoved,
	"delete":  errDbtDeleteRemoved,
	"cleanup": errDbtCleanupRemoved,
}
