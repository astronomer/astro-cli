package astro

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/pkg/util"
)

// --deployment and --workspace are the spellings help shows, and the ones
// commands read. On a command 1.x had, the ones they replace, --deployment-id,
// --deployment-name and --workspace-id, keep working as they always have,
// hidden, with the shorthands 1.x gave them: the command opts in to them
// (oneXSpelling), and they are aliases that applyPreferredFlags hands to
// --deployment or --workspace, which nothing else reads.

// deploymentTargets is where a command takes the Deployment --deployment
// names: id for a Deployment id, name for a Deployment name, either nil on a
// command that takes no such thing. scope is true where the Deployment is the
// command's scope in place of a workspace (astro env), so a project's
// workspace has no say once one is named.
type deploymentTargets struct {
	id, name *string
	scope    bool
}

// workspaceChoiceAnnotation marks a flag that chooses the workspace a command
// acts in (--workspace, deployment list's --all), so a project's workspace
// yields to it once given.
const workspaceChoiceAnnotation = "astro.workspace-choice"

// markWorkspaceChoice marks f as choosing the command's workspace.
func markWorkspaceChoice(f *pflag.Flag) {
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[workspaceChoiceAnnotation] = nil
}

// workspaceChosen reports whether this run gave a flag that chooses the
// command's workspace.
func workspaceChosen(fs *pflag.FlagSet) bool {
	chosen := false
	fs.Visit(func(f *pflag.Flag) {
		if _, ok := f.Annotations[workspaceChoiceAnnotation]; ok {
			chosen = true
		}
	})
	return chosen
}

// oneXSpelling is a 1.x flag a command that 1.x had keeps as a hidden alias of
// its v2 spelling, with the shorthand 1.x gave it ("" for none). A command
// opts in to each one; a command 1.x did not have takes none.
type oneXSpelling struct {
	name, shorthand string
}

// oneXWorkspaceID is 1.x's --workspace-id, which had no shorthand.
var oneXWorkspaceID = oneXSpelling{name: "workspace-id"}

// oneXDeploymentID is 1.x's --deployment-id, with the shorthand it had on
// the command.
func oneXDeploymentID(shorthand string) oneXSpelling {
	return oneXSpelling{name: "deployment-id", shorthand: shorthand}
}

// oneXDeploymentName is 1.x's --deployment-name, with the shorthand it had on
// the command.
func oneXDeploymentName(shorthand string) oneXSpelling {
	return oneXSpelling{name: "deployment-name", shorthand: shorthand}
}

// deploymentFlag is the value of --deployment. It splits what it was given
// into the command's id or name once the flags are parsed (resolve), and
// records which of the two this run named, for followProject.
type deploymentFlag struct {
	value string
	deploymentTargets
	// idAlias and nameAlias are the hidden --deployment-id and
	// --deployment-name; a value given to one keeps the kind it was given as.
	idAlias, nameAlias *pflag.Flag
	// self is --deployment.
	self *pflag.Flag
	// givenID and givenName are what resolve set this run, and arg a
	// Deployment id given as --deployment to a command that takes one only as
	// its argument, which followProject reads as it reads that argument.
	givenID, givenName bool
	arg                string
}

func (d *deploymentFlag) String() string     { return d.value }
func (d *deploymentFlag) Set(v string) error { d.value = v; return nil }
func (d *deploymentFlag) Type() string       { return "string" }

// addDeploymentFlag registers --deployment on fs, splitting into to's id and
// name, and the 1.x spellings oneX names as hidden aliases: --deployment-id
// for a command with an id, --deployment-name for one with a name.
func addDeploymentFlag(fs *pflag.FlagSet, usage string, to deploymentTargets, oneX ...oneXSpelling) {
	d := &deploymentFlag{deploymentTargets: to}
	fs.Var(d, "deployment", usage)
	d.self = fs.Lookup("deployment")
	utils.RefuseEmpty(d.self)
	if to.id != nil {
		*to.id = ""
	}
	if to.name != nil {
		*to.name = ""
	}
	for _, o := range oneX {
		switch {
		case o.name == "deployment-id" && to.id != nil:
			d.idAlias = utils.AddAlias(fs, o.name, o.shorthand, "deployment")
		case o.name == "deployment-name" && to.name != nil:
			d.nameAlias = utils.AddAlias(fs, o.name, o.shorthand, "deployment")
		default:
			panic(fmt.Sprintf("addDeploymentFlag: --%s is no 1.x spelling of what this --deployment takes", o.name))
		}
	}
}

// addWorkspaceFlag registers --workspace on fs, read into to, and the 1.x
// spelling oneX names (oneXWorkspaceID) as a hidden alias.
func addWorkspaceFlag(fs *pflag.FlagSet, to *string, shorthand, usage string, oneX ...oneXSpelling) {
	fs.StringVarP(to, "workspace", shorthand, "", usage)
	utils.RefuseEmpty(fs.Lookup("workspace"))
	markWorkspaceChoice(fs.Lookup("workspace"))
	for _, o := range oneX {
		if o != oneXWorkspaceID {
			panic(fmt.Sprintf("addWorkspaceFlag: --%s is no 1.x spelling of --workspace", o.name))
		}
		utils.AddAlias(fs, o.name, o.shorthand, "workspace")
	}
}

// oneXNoOpUsage is the help of a 1.x flag a command accepts and ignores.
const oneXNoOpUsage = "Has no effect; accepted so scripts written for Astro CLI 1.x keep working"

// addOneXWorkspaceIDNoOp registers 1.x's --workspace-id on fs for a command
// that 1.x gave it to, through its group, and that never read it: hidden and
// read by nothing, so a 1.x script passing it keeps working. It is no
// spelling of --workspace, which the command does not take, and chooses no
// workspace, so a project's workspace and host apply as they would without
// it.
func addOneXWorkspaceIDNoOp(fs *pflag.FlagSet) {
	fs.String(oneXWorkspaceID.name, "", oneXNoOpUsage)
	fs.MarkHidden(oneXWorkspaceID.name) //nolint:errcheck // the flag is defined just above
}

// deploymentWorkspaceUsage is --workspace on a deployment command that looks
// its Deployment up in a workspace.
const deploymentWorkspaceUsage = "Workspace the Deployment is in (default: the project's workspace inside a project with a pyproject.toml, else the current one)"

// addDeploymentWorkspaceFlag registers --workspace, and 1.x's --workspace-id
// as its alias, on a deployment command that reads it.
func addDeploymentWorkspaceFlag(fs *pflag.FlagSet) {
	addWorkspaceFlag(fs, &workspaceID, "", deploymentWorkspaceUsage, oneXWorkspaceID)
}

// deploymentFlagOf is the --deployment on fs that addDeploymentFlag
// registered, or nil.
func deploymentFlagOf(fs *pflag.FlagSet) *deploymentFlag {
	var d *deploymentFlag
	fs.VisitAll(func(f *pflag.Flag) {
		if v, ok := f.Value.(*deploymentFlag); ok {
			d = v
		}
	})
	return d
}

// applyPreferredFlagsIn has every runnable command in tree reconcile its
// spellings before its argument check and its pre-runs, which read what they
// resolve to. Each top-level constructor calls it on its own tree.
func applyPreferredFlagsIn(tree *cobra.Command) {
	utils.BeforeArgs(tree, applyPreferredFlags)
}

// applyPreferredFlags is the hook every runnable Astro command runs once its
// flags are parsed: the 1.x spellings go to the flags they stand for, and
// --deployment is split into the command's Deployment id or name.
func applyPreferredFlags(cmd *cobra.Command, args []string) error {
	fs := cmd.Flags()
	d := deploymentFlagOf(fs)
	typed := d != nil && d.self.Changed
	if err := utils.ApplyAliases(fs); err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	d.resolve(typed)
	// A command whose id is its argument would take the argument over a
	// --deployment id without a word, and `delete <a> --deployment <b>` would
	// delete a. Two different Deployments named at once is a usage mistake,
	// the same as an old and a new spelling that disagree.
	if d.arg != "" && len(args) > 0 && args[0] != d.arg {
		return cliout.Usage(fmt.Errorf("the Deployment argument %q and --deployment %q disagree: pass only one", args[0], d.arg))
	}
	return nil
}

// resolve splits what this run gave into the command's id and name. A value
// given as --deployment-id is an id and one given as --deployment-name a
// name, as in 1.x, whatever it looks like; both may be given, as 1.x read
// both. A value typed as --deployment resolves in the order the CLI already
// uses: a link name first (followProject reads one from either), then a
// Deployment id, then a Deployment name. An id goes to the id and anything
// else to the name, where the command takes both; a command with one takes
// everything there, except an id on a command whose id is its argument.
func (d *deploymentFlag) resolve(typed bool) {
	d.givenID, d.givenName, d.arg = false, false, ""
	if d.idAlias != nil && d.idAlias.Changed {
		*d.id, d.givenID = d.idAlias.Value.String(), true
	}
	if d.nameAlias != nil && d.nameAlias.Changed {
		*d.name, d.givenName = d.nameAlias.Value.String(), true
	}
	if !typed {
		return
	}
	v := d.value
	switch {
	case d.id != nil && (d.name == nil || util.IsCUID(v)):
		*d.id, d.givenID = v, true
	case d.name != nil && !util.IsCUID(v):
		*d.name, d.givenName = v, true
	default:
		d.arg = v
		deploymentID = v
	}
}
