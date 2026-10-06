package astro

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/pkg/util"
)

// --deployment and --workspace are the spellings help shows. The ones they
// replace, --deployment-id, --deployment-name and --workspace-id, keep working
// as they always have, hidden: commands go on reading those, and
// applyPreferredFlags hands them what --deployment or --workspace was given.

// deploymentArg is a Deployment id given as --deployment to a command that
// takes a Deployment's id only as its argument, and a name as
// --deployment-name. followProject reads it as it reads that argument.
var deploymentArg string

// addDeploymentFlag registers --deployment beside whichever of
// --deployment-id and --deployment-name the command already has on fs.
func addDeploymentFlag(fs *pflag.FlagSet, usage string) {
	var olds []string
	for _, name := range []string{"deployment-id", "deployment-name"} {
		if fs.Lookup(name) != nil {
			olds = append(olds, name)
		}
	}
	utils.AddPreferredFlag(fs, "deployment", "", usage, olds...)
}

// addWorkspaceFlag registers --workspace beside the --workspace-id on fs.
func addWorkspaceFlag(fs *pflag.FlagSet, shorthand, usage string) {
	utils.AddPreferredFlag(fs, "workspace", shorthand, usage, "workspace-id")
}

// applyPreferredFlagsIn has every runnable command in tree reconcile its
// preferred flags before its argument check and its pre-runs, which read the
// older spellings. Each top-level constructor calls it on its own tree.
func applyPreferredFlagsIn(tree *cobra.Command) {
	utils.BeforeArgs(tree, applyPreferredFlags)
}

// applyPreferredFlags is the hook every runnable Astro command runs once its
// flags are parsed.
func applyPreferredFlags(cmd *cobra.Command, args []string) error {
	deploymentArg = ""
	if err := utils.ApplyPreferredFlags(cmd.Flags(), routeDeployment); err != nil {
		return err
	}
	// A command whose id is its argument would take the argument over a
	// --deployment id without a word, and `delete <a> --deployment <b>` would
	// delete a. Two different Deployments named at once is a usage mistake,
	// the same as an old and a new spelling that disagree.
	if deploymentArg != "" && len(args) > 0 && args[0] != deploymentArg {
		return cliout.Usage(fmt.Errorf("the Deployment argument %q and --deployment %q disagree: pass only one", args[0], deploymentArg))
	}
	return nil
}

// routeDeployment sends a --deployment value where the older spellings would
// have taken it, so it resolves in the order the CLI already uses: a link
// name first (followProject reads one from either flag), then a Deployment
// id, then a Deployment name. An id goes to --deployment-id and anything else
// to --deployment-name, where the command has both; a command with one takes
// everything there, except an id on a command whose id is its argument.
func routeDeployment(fs *pflag.FlagSet, preferred, v string) (string, bool) {
	if preferred != "deployment" {
		return "", false
	}
	hasID := fs.Lookup("deployment-id") != nil
	hasName := fs.Lookup("deployment-name") != nil
	switch {
	case hasID && (!hasName || util.IsCUID(v)):
		return "deployment-id", true
	case hasName && !util.IsCUID(v):
		return "deployment-name", true
	}
	deploymentArg = v
	deploymentID = v
	return "", true
}

// addJSONFlag registers 1.x's --json on cmd as a hidden spelling of
// --output json. It is applied in the command's argument check, which runs
// ahead of every pre-run, so a failure before the command runs is reported
// as json too. Given beside an --output that is not json, the two disagree,
// which is a usage error like any old and new spelling that disagree.
func addJSONFlag(cmd *cobra.Command) {
	var asJSON bool
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON, the same as --output json")
	_ = cmd.Flags().MarkHidden("json") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
	check := cmd.Args
	cmd.Args = func(c *cobra.Command, args []string) error {
		if asJSON {
			json := string(cliout.FormatJSON)
			if o := c.Flag("output"); o.Changed && o.Value.String() != json {
				return cliout.Usage(fmt.Errorf("--json and --output %q disagree: pass only --output", o.Value.String()))
			}
			if err := c.Flags().Set("output", json); err != nil {
				return err
			}
		}
		if check == nil {
			return nil
		}
		return check(c, args)
	}
}
