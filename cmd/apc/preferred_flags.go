package apc

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// --deployment and --workspace are the spellings help shows, and the ones
// commands read, as on Astro. On a command 1.x had, the ones they replace,
// --deployment-id and --workspace-id, keep working as they always have,
// hidden: utils.AddAlias registers them, and applyPreferredFlags hands what
// they were given to the v2 flag, which is all the command reads. A
// shorthand 1.x gave the old spelling (-d, -w) belongs to the v2 one now.

// applyPreferredFlagsIn has every runnable command in tree reconcile its
// spellings before its argument check and its pre-runs. Each top-level
// constructor calls it on its own tree.
func applyPreferredFlagsIn(tree *cobra.Command) {
	utils.BeforeArgs(tree, applyPreferredFlags)
}

// deploymentFlag is the value of --deployment, which every APC command reads
// as deploymentID: a link name of the project around the working directory,
// or a Deployment id, which applyPreferredFlags turns into the id.
type deploymentFlag struct {
	// positional is set on a command that takes the Deployment id as its
	// argument too: either names it, and given both they must agree.
	positional bool
}

func (d *deploymentFlag) String() string     { return deploymentID }
func (d *deploymentFlag) Set(v string) error { deploymentID = v; return nil }
func (d *deploymentFlag) Type() string       { return "string" }

// addDeploymentFlag registers --deployment on fs, with the shorthand given
// ("" for none), and with oneXID 1.x's --deployment-id as a hidden alias,
// which takes an id alone, as in 1.x. It returns the flag's value.
func addDeploymentFlag(fs *pflag.FlagSet, shorthand, usage string, oneXID bool) *deploymentFlag {
	deploymentID = ""
	d := &deploymentFlag{}
	fs.VarP(d, "deployment", shorthand, usage)
	utils.RefuseEmpty(fs.Lookup("deployment"))
	if oneXID {
		utils.AddAlias(fs, "deployment-id", "", "deployment")
	}
	return d
}

// addDeploymentArgFlag registers --deployment on a command that takes the
// Deployment id as its argument, which stays: either names the Deployment.
// 1.x took it as the argument alone, so there is no 1.x spelling to keep.
func addDeploymentArgFlag(cmd *cobra.Command) {
	addDeploymentFlag(cmd.Flags(), "", "Deployment: a link name from the project's manifest, or a Deployment id", false).positional = true
}

// addWorkspaceFlag registers --workspace on fs, read into workspaceID, with
// the shorthand given ("" for none), and 1.x's --workspace-id as its hidden
// alias, without a shorthand: one 1.x gave it is --workspace's now. Every APC
// command with a --workspace had --workspace-id in 1.x; one that did not
// would register --workspace alone.
func addWorkspaceFlag(fs *pflag.FlagSet, shorthand, usage string) {
	fs.StringVarP(&workspaceID, "workspace", shorthand, "", usage)
	utils.RefuseEmpty(fs.Lookup("workspace"))
	utils.AddAlias(fs, "workspace-id", "", "workspace")
}

// deploymentFlagOf is the --deployment on fs that addDeploymentFlag
// registered, or nil.
func deploymentFlagOf(fs *pflag.FlagSet) *deploymentFlag {
	if f := fs.Lookup("deployment"); f != nil {
		if d, ok := f.Value.(*deploymentFlag); ok {
			return d
		}
	}
	return nil
}

// applyPreferredFlags is the hook every runnable APC command runs once its
// flags are parsed: the 1.x spellings go to the flags they stand for (an
// empty one meaning the default, as in 1.x, an empty v2 one refused), and a
// --deployment typed as a link name becomes the link's Deployment id. A value
// given as --deployment-id is an id, as in 1.x, whatever it looks like. On a
// command that takes the Deployment as its argument too, the argument stays
// an id, as it always was; given with --deployment, the two must name the
// same Deployment. Given neither, the command refuses the run
// (requireDeployment) when it runs, not here: an argument check judges the
// arguments a line could take, and a help page's example is one.
func applyPreferredFlags(cmd *cobra.Command, args []string) error {
	fs := cmd.Flags()
	d := deploymentFlagOf(fs)
	typed := d != nil && fs.Changed("deployment")
	if err := utils.ApplyAliases(fs); err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	given := deploymentID
	if typed {
		id, err := resolveDeployment(given)
		if err != nil {
			return err
		}
		deploymentID = id
	}
	if !d.positional {
		return nil
	}
	switch {
	case len(args) == 0:
		return nil
	case typed && args[0] != given && args[0] != deploymentID:
		return cliout.Usage(fmt.Errorf("the Deployment argument %q and --deployment %q disagree: pass only one", args[0], given))
	}
	deploymentID = args[0]
	return nil
}

// requireDeployment refuses a run of a command that takes its Deployment as
// the argument or as --deployment, and was given neither.
func requireDeployment(cmd *cobra.Command) error {
	if deploymentID != "" {
		return nil
	}
	return cliout.Usage(fmt.Errorf("name the Deployment: %s <DEPLOYMENT_ID>, or --deployment <LINK_OR_DEPLOYMENT_ID>", cmd.CommandPath()))
}

// resolveDeployment is the Deployment id --deployment names: an apc link of
// the project around the working directory, on the current context's domain,
// resolved as astro deploy resolves one, else the value itself, a Deployment
// id. Outside a project there is no link to read.
func resolveDeployment(name string) (string, error) {
	where := utils.Locate(config.WorkingPath)
	if where.LoadErr != nil {
		return "", where.LoadErr
	}
	if where.Manifest == nil {
		return name, nil
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}
	target, _, err := manifestdeploy.ResolveNamed(manifestdeploy.Request{
		Manifest:    where.Manifest,
		Deployment:  name,
		Platform:    manifest.KindAPC,
		Domain:      manifest.NormalizeAPCDomain(c.Domain),
		OtherDomain: otherDomainThen("run the command again"),
	})
	if err != nil {
		return "", cliout.Usage(err)
	}
	return target.DeploymentID, nil
}
