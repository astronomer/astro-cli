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
// hidden, with the shorthand 1.x gave them (-d, -w): utils.AddAlias
// registers them, and the hook utils.ApplyPreferredFlagsIn installs hands
// what they were given to the v2 flag, which is all a command reads. The v2
// spellings take no shorthand here, so a 1.x shorthand keeps meaning exactly
// what it did.

// applyPreferredFlagsIn has every runnable command in tree reconcile its
// spellings before its argument check and its pre-runs, with the APC tree's
// resolution of --deployment (resolveDeploymentFlag). Each top-level
// constructor calls it on its own tree.
func applyPreferredFlagsIn(tree *cobra.Command) {
	utils.ApplyPreferredFlagsIn(tree, resolveDeploymentFlag)
}

// deploymentLink is the link --deployment named this run, "" when it named a
// Deployment id, or was not given.
var deploymentLink string

// deploymentFlag is the value of --deployment, which every APC command reads
// as deploymentID: a link name of the project around the working directory,
// or a Deployment id, which resolveDeploymentFlag turns into the id.
type deploymentFlag struct {
	// positional is set on a command that takes the Deployment id as its
	// argument too: either names it, and given both they must agree.
	positional bool
}

func (d *deploymentFlag) String() string     { return deploymentID }
func (d *deploymentFlag) Set(v string) error { deploymentID = v; return nil }
func (d *deploymentFlag) Type() string       { return "string" }

// addDeploymentFlag registers --deployment on fs, with no shorthand, and with
// oneX 1.x's --deployment-id as its hidden alias, with the shorthand 1.x gave
// it there (oneXShorthand, "" for none). The alias takes an id alone, as in
// 1.x. It returns the flag's value.
func addDeploymentFlag(fs *pflag.FlagSet, usage string, oneX bool, oneXShorthand string) *deploymentFlag {
	deploymentID = ""
	d := &deploymentFlag{}
	fs.Var(d, "deployment", usage)
	utils.RefuseEmpty(fs.Lookup("deployment"))
	if oneX {
		utils.AddAlias(fs, "deployment-id", oneXShorthand, "deployment")
	}
	return d
}

// addDeploymentArgFlag registers --deployment on a command that takes the
// Deployment id as its argument, which stays: either names the Deployment.
// 1.x took it as the argument alone, so there is no 1.x spelling to keep.
func addDeploymentArgFlag(cmd *cobra.Command) {
	addDeploymentFlag(cmd.Flags(), "Deployment: a link name from the project's manifest, or a Deployment id", false, "").positional = true
}

// addWorkspaceFlag registers --workspace on fs, read into workspaceID, with
// no shorthand, and 1.x's --workspace-id as its hidden alias, with the
// shorthand 1.x gave it there (oneXShorthand, "" for none). Every APC command
// with a --workspace had --workspace-id in 1.x.
func addWorkspaceFlag(fs *pflag.FlagSet, usage, oneXShorthand string) {
	utils.AddWorkspaceFlag(fs, &workspaceID, "", usage, true, oneXShorthand)
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

// resolveDeploymentFlag is the APC tree's work once the 1.x spellings are
// handed on: a --deployment typed as a link name becomes the link's
// Deployment id. A value given as --deployment-id is an id, as in 1.x,
// whatever it looks like. On a command that takes the Deployment as its
// argument too, the argument stays an id, as it always was; given with
// --deployment, the two must name the same Deployment. Given neither, the
// command refuses the run (requireDeployment) when it runs, not here: an
// argument check judges the arguments a line could take, and a help page's
// example is one.
func resolveDeploymentFlag(cmd *cobra.Command, args []string, typed func(string) bool) error {
	deploymentLink = ""
	d := deploymentFlagOf(cmd.Flags())
	if d == nil {
		return nil
	}
	if !typed("deployment") {
		if d.positional && len(args) > 0 {
			deploymentID = args[0]
		}
		return nil
	}
	given := deploymentID
	id, link, err := resolveDeployment(cmd, given)
	if err != nil {
		return err
	}
	deploymentID, deploymentLink = id, link
	if !d.positional {
		return nil
	}
	return utils.RefuseDisagreeingDeployment(args, given, id)
}

// requireDeployment refuses a run of a command that takes its Deployment as
// the argument or as --deployment, and was given neither.
func requireDeployment(cmd *cobra.Command) error {
	if deploymentID != "" {
		return nil
	}
	return cliout.Usage(fmt.Errorf("name the Deployment: %s <DEPLOYMENT_ID>, or --deployment <LINK_OR_DEPLOYMENT_ID>", cmd.CommandPath()))
}

// resolveDeployment is the Deployment id --deployment names, and the link
// that named it: an apc link of the project around the working directory, on
// the current context's domain, resolved as astro deploy resolves one, else
// the value itself, a Deployment id, and no link. Outside a project there is
// no link to read. A link of another platform is refused in cmd's words.
func resolveDeployment(cmd *cobra.Command, name string) (id, link string, err error) {
	where := utils.Locate(config.WorkingPath)
	if where.LoadErr != nil {
		return "", "", where.LoadErr
	}
	m := where.Manifest
	if m == nil {
		return name, "", nil
	}
	if l, ok := m.Astro.Deployments[name]; ok && l.Kind() != manifest.KindAPC {
		return "", "", cliout.Usage(fmt.Errorf("link %q is %s, and %s acts on Astro Private Cloud Deployments: name an apc link or a Deployment id", name, l.Kind(), cmd.CommandPath()))
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", "", err
	}
	target, _, err := manifestdeploy.ResolveNamed(manifestdeploy.Request{
		Manifest:    m,
		Deployment:  name,
		Platform:    manifest.KindAPC,
		Domain:      manifest.NormalizeAPCDomain(c.Domain),
		OtherDomain: otherDomainThen("run the command again"),
	})
	if err != nil {
		return "", "", cliout.Usage(err)
	}
	return target.DeploymentID, target.LinkName, nil
}
