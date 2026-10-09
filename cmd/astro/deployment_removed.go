package astro

import (
	"fmt"

	"github.com/spf13/cobra"
)

// removedDeploymentObject is one of the groups that wrote Airflow objects
// straight into a Deployment's metadata database through the Airflow REST
// API. The Environment Manager replaced them: `astro env` holds connections
// and Airflow variables at workspace or Deployment scope.
type removedDeploymentObject struct {
	noun    string
	aliases []string
	// envNoun is the `astro env` group that replaces it, empty when none does
	// yet.
	envNoun string
}

// Aliases are the ones each group carried, so every old spelling reaches the
// guidance rather than cobra's help-and-exit-0 for an unknown subcommand.
var removedDeploymentObjects = []removedDeploymentObject{
	{noun: "connection", aliases: []string{"con", "connections"}, envNoun: "connection"},
	{noun: "airflow-variable", aliases: []string{"airflow-var", "airflow-vars", "airflow-variables"}, envNoun: "airflow-variable"},
	{noun: "pool", aliases: []string{"pl", "pools"}},
}

func newRemovedDeploymentObjectCmds() []*cobra.Command {
	cmds := make([]*cobra.Command, 0, len(removedDeploymentObjects))
	for _, o := range removedDeploymentObjects {
		cmds = append(cmds, newRemovedDeploymentObjectCmd(o))
	}
	return cmds
}

// newRemovedDeploymentObjectCmd builds the tombstone for one group: one stub
// (removedCmdStub) taking any subcommand, so an old invocation's
// --deployment-id or --conn-id reaches the guidance instead of dying on the
// flag. Its Short says in brief what the guidance says.
func newRemovedDeploymentObjectCmd(o removedDeploymentObject) *cobra.Command {
	short := "Removed in v2 — use the Airflow UI or the Airflow REST API"
	if o.envNoun != "" {
		short = "Removed in v2 — use `astro env " + o.envNoun + "`"
	}
	return removedCmdStub(o.noun, o.aliases, short, func(args []string) string {
		return removedDeploymentObjectGuidance(o, args)
	})
}

// removedDeploymentObjectGuidance names the replacement for the verb typed,
// which is the first argument when there is one.
func removedDeploymentObjectGuidance(o removedDeploymentObject, args []string) string {
	head := fmt.Sprintf("`astro deployment %s` was removed in Astro CLI v2.\n", o.noun)
	if o.envNoun == "" {
		return head + fmt.Sprintf("It has no replacement in the CLI yet: Airflow %ss are not in the Environment Manager yet. "+
			"Until they are, manage them in the Airflow UI or through the Airflow REST API.", o.noun)
	}

	verb := ""
	if len(args) > 0 {
		verb = args[0]
	}
	envCmd := "astro env " + o.envNoun
	switch verb {
	case "list", "li":
		return head + fmt.Sprintf("  use:  %s list --deployment <deployment-id>", envCmd)
	case "create", "cr", "update", "up":
		return head + fmt.Sprintf("  use:  %s set <key> --deployment <deployment-id>\n"+
			"`set` creates the object when it does not exist and updates it when it does.", envCmd)
	case "delete", "rm":
		return head + fmt.Sprintf("  use:  %s delete <key> --deployment <deployment-id>", envCmd)
	case "copy", "cp":
		return head + fmt.Sprintf("There is no copy. To share one object between Deployments, set it once in the workspace:\n"+
			"  use:  %s set <key> --workspace <workspace-id> --auto-link\n"+
			"or set it on each Deployment with --deployment.", envCmd)
	}
	return head + fmt.Sprintf("  use:  %s --help\n"+
		"The Environment Manager holds %ss at workspace or Deployment scope.", envCmd, o.noun)
}
