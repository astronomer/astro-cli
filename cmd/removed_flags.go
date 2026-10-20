package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
)

// terraformProviderURL is where Deployments as code live now that the CLI no
// longer reads deployment files.
const terraformProviderURL = "https://registry.terraform.io/providers/astronomer/astro/latest"

// What a run passing a removed 1.x flag is told after "<flag> was removed in
// Astro CLI v2: ", which removedMessage puts before each, with the flag as the
// run typed it.
const (
	errJSONFlagRemoved     = "use -o json"
	errTemplateFlagRemoved = "use -o json, and a JSON tool such as jq to pick fields"
	errAPIURLFlagRemoved   = "use --url"
	errDeploymentIDAPIFlag = "use --deployment (-d), which takes a Deployment id or a link name from pyproject.toml"

	// astro env list is new in v2 and takes only the v2 spellings, but a
	// script carrying the per-kind lists' 1.x habits may still type these.
	errEnvListWorkspaceIDRemoved  = "use --workspace"
	errEnvListDeploymentIDRemoved = "use --deployment, which takes a Deployment id or a link name from pyproject.toml"

	// astro deployment team, user and token took a --workspace in v2 before
	// it moved to the commands that read it; none of these ever read it.
	errDeploymentByIDWorkspaceRemoved = "astro deployment team, user and token act on the Deployment you name, which carries its workspace, so drop it"

	errDeploymentFileRemoved = "to manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL + ". " +
		"To copy a Deployment, use astro deployment create --clone <deployment> --name <new name>"
	errInspectTemplateRemoved = "to copy a Deployment, use astro deployment create --clone <deployment> --name <new name>. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL
	// A switch has not logged in again since 2023, so there is no login to
	// link to.
	errLoginLinkRemoved = "switching organizations no longer re-authenticates. " +
		"To log in on another device, use astro login --login-link"

	// astro deploy ships only pyproject.toml projects in v2, so the flags only
	// the 1.x project's deploy read went with it. A 1.x project deploys with
	// Astro CLI 1.x, flags and all.
	errDeploySaveRemoved = "astro deploy asks which Deployment to deploy to unless you name one, as the argument or with --deployment. " +
		"To preselect one in that list, link it with astro link add and mark it default = true under [tool.astro.deployments]"
	errDeployPytestRemoved     = "run your tests before deploying, with uv run pytest && astro deploy"
	errDeployTestRemoved       = "run your tests before deploying, with uv run pytest <path> && astro deploy"
	errDeployEnvRemoved        = "astro deploy runs no tests, so it reads no test env file. Run your tests with uv run pytest before deploying"
	errDeployParseRemoved      = "check your Dags before deploying, with astro local check && astro deploy"
	errDeployDagsPathRemoved   = "astro deploy --dags ships the project's dags directory, so run it from the project"
	errDeployBundleNameRemoved = "deploying to a named Dag bundle is not supported yet. Use Astro CLI 1.x for it"
	errDeployNameRemoved       = "name the Deployment as the argument or with --deployment, which take a link name or a Deployment id"
	// astro deploy --non-dags became astro deploy bundle, a subcommand of
	// its own with the bundle's directory as its argument.
	errDeployNonDagsRemoved = "use astro deploy bundle [PATH] --deployment <Deployment id, name or link name>, " +
		"which deploys a non-Dag bundle such as a dbt project from its directory"
	errDeployNonDagsMountPathRemoved  = "use astro deploy bundle --mount-path"
	errDeployNonDagsLocalPathRemoved  = "pass the bundle's directory as the argument, astro deploy bundle <PATH>"
	errDeployNonDagsBundleTypeRemoved = "use astro deploy bundle --type, which detects a dbt project by itself"
	// 1.x kept --build-secrets, hidden and deprecated, as an alias of the
	// repeatable --build-secret that replaced it.
	errBuildSecretsRemoved = "use --build-secret, once for each secret" //nolint:gosec // message text naming a flag, not a credential
	errAPCDeployNoCache    = "astro deploy " + buildsOnTheEngineCache
	// buildsOnTheEngineCache is what a command that builds an image says of
	// --no-cache, after its own name.
	buildsOnTheEngineCache = "builds on the container engine's cache, which a changed dependency or file invalidates. " +
		"To build from nothing, clear the engine's build cache first, with docker builder prune"

	// The flags 1.x marked deprecated, hidden but accepted, which v2 dropped.
	errEnforceCICDRemoved      = "use --cicd-enforcement enable, or --cicd-enforcement disable"
	errClusterTypeRemoved      = "use --type, which takes the same standard or dedicated"
	errForceUpgradeToAF3Remove = "a deploy that moves a Deployment from Airflow 2 to Airflow 3 needs no flag, " +
		"as it needed none in 1.x, which ignored this one, so drop it"
	errHardDeleteRemoved = "deleting a Deployment always removes all of its infrastructure and records, " +
		"as it did in 1.x, which ignored --hard, so drop it"

	// astro dev init's flags, on astro init.
	errInitForceRemoved    = "astro init asks nothing before it writes, in an empty directory or not, so drop it"
	errInitTemplateRemoved = "there are no templates to choose from, and astro init writes one starter project, with an example Dag in dags/"
	errInitRuntimeRemoved  = "a project pins Airflow, with --airflow-version or the apache-airflow requirement in pyproject.toml, " +
		"and Docker mode builds on the newest Astro Runtime for it. To pin one Runtime build, set runtime = \"<version>\" under [tool.astro] in pyproject.toml"
	errInitRemoteExecRemoved = "astro init writes no client image files. " +
		"To deploy a client image for remote execution, add a Dockerfile.client to the project and use astro remote deploy"
	errInitRemoteRepoRemoved = "set the repository astro remote deploy pushes the client image to with " +
		"astro config set deploy.remote_execution.client_registry <registry>/<repository>"
	errInitACRemoved = "a project runs Apache Airflow, on Astro Runtime in Docker mode, " +
		"and there is no Astronomer Certified image to choose. Pin Airflow with --airflow-version"
	errInitDockerRemoved     = "astro init makes one project for both modes. Choose Docker when you start it, with astro local start --docker"
	errInitStandaloneRemoved = "astro init makes one project for both modes, and astro local start runs it standalone by default, so drop it"

	// astro dev start's and restart's flags, on astro local start and
	// restart (and the root's astro start).
	errStartStandaloneRemoved  = "standalone is the default mode, so drop it"
	errStartComposeFileRemoved = "Docker mode writes its own compose file, and merges a docker-compose.override.yml " +
		"at the project root over it. Put your changes there"
	errStartDeploymentIDRemoved = "a start reads no Deployment's values. " +
		"It reads the Environment Manager of the workspace pyproject.toml names, with workspace = \"<workspace id>\" under [tool.astro]"
	errStartWorkspaceIDRemoved = "a start reads the Environment Manager of the workspace pyproject.toml names, " +
		"with workspace = \"<workspace id>\" under [tool.astro]"
	errStartImageNameRemoved = "Docker mode builds the project's image itself, from pyproject.toml, " +
		"or from the Dockerfile that dockerfile = \"<path>\" under [tool.astro] names"
	errStartNoBrowserRemoved = "a start opens no browser, so drop it. To open the Airflow UI, use astro local open"
	errStartNoProxyRemoved   = "Airflow always answers on its own port on localhost too, the URL astro local status shows as direct. " +
		"To choose that port, use astro local start --port"
	errStartSettingsRemoved = "a start sets the connections, variables and pools pyproject.toml declares, " +
		"in [tool.astro.env] and [tool.astro.pools]. astro init carries an airflow_settings.yaml into them"
	errRestartPortRemoved = "astro local restart keeps the port Airflow is running on. " +
		"To change it, use astro local stop, then astro local start --port <port>"
	errRestartKillRemoved = "to wipe the local database and logs first, use astro local reset --yes, then astro local start"

	// astro dev build's, on astro package.
	errPackageImageNameRemoved = "astro package always builds the image, from pyproject.toml, " +
		"or from the Dockerfile that dockerfile = \"<path>\" under [tool.astro] names. To name the image it builds, use --tag"
)

// removedFlag is what a run passing a flag Astro CLI 1.x had and v2 dropped
// is told instead of cobra's bare "unknown flag".
type removedFlag struct {
	name string
	// under limits the entry to these commands and everything below them,
	// each a command path without the root's name ("api airflow"), for a
	// name whose replacement differs by command. Empty means any command.
	under []string
	// needs reports whether what the message tells the user to use exists on
	// cmd. nil means it always does.
	needs func(cmd *cobra.Command) bool
	// reason is what replaced the flag on cmd, or why nothing did, given the
	// flag's name, for one reason several entries share. It follows the flag
	// as the run typed it (removedMessage), so it never names that itself.
	reason reasonFunc
	// v2Only is set on an entry for a flag 1.x did not have on the commands
	// under names: a 1.x spelling a script may carry over to a v2 command
	// that never took it, or a spelling an earlier v2 build gave a command
	// and took away. v1_flags.tsv has no row for it there, so it is
	// consulted without one, for the long name alone.
	v2Only bool
}

// reasonFunc builds a removedFlag's reason.
type reasonFunc func(cmd *cobra.Command, name string) string

// says is the reasonFunc of a reason that is the same wherever it applies.
func says(reason string) reasonFunc {
	return func(*cobra.Command, string) string { return reason }
}

// message is what a run that typed the flag as typed ("-f" or "--force") is
// told on cmd.
func (f *removedFlag) message(cmd *cobra.Command, typed string) string {
	return removedMessage(typed, f.reason(cmd, f.name))
}

// removedMessage is what a run that typed a removed 1.x flag as typed is
// told: led by the flag as the run spelled it, so `-s` is not answered about
// a --scheduler it never wrote.
func removedMessage(typed, reason string) string {
	return typed + " was removed in Astro CLI v2: " + reason
}

// removedFlags is every 1.x flag v2 removed without keeping a hidden alias,
// in one place: the list is defined by 1.x rather than by any one command, so
// it is reviewed against 1.x's tree (v1_flags.tsv) and deleted in v3 as one.
//
// It is consulted only when cobra reports a flag the invoked command does not
// have (removedFlagError, from the root's flag error func), and only for a
// flag v1_flags.tsv says 1.x had on that command, its own or inherited, or on
// the 1.x command it replaced under another name (v1Predecessors: astro dev
// start's flags on astro local start). So a command that still has a flag of
// the same name (`astro deploy --force`) is untouched, a command 1.x did not
// have and that replaced none, or had without the flag, gets cobra's error
// (`astro local reset --force`: astro dev kill had no --force), and a
// shorthand 1.x gave a flag the command still has under its long name alone
// names that (shorthandRemoved). No command carries a hidden flag for it (except while
// a shell asks for completions: acceptRemovedFlags). An entry is keyed by the
// flag's name, and a shorthand 1.x gave it reaches it through v1_flags.tsv:
// 1.x's --force (-f) became --yes on every command that asks first, so the
// one entry says so on each. Where one name had different replacements on
// different commands, the narrower entry (with under) comes first, and the
// first entry that applies wins. An entry marked v2Only is the exception: it
// names a v2 command 1.x did not have (astro env list) and answers there for a
// 1.x spelling a script may carry over, with no row of v1_flags.tsv behind it.
//
// A future removal belongs here too: TestEveryV1FlagStillWorksOrSaysWhatReplacedIt
// fails on a flag in v1_flags.tsv that the v2 tree neither has nor reports
// here, and on an entry no flag in it reaches.
var removedFlags = []removedFlag{
	// Deployments as code moved to Terraform.
	{name: "deployment-file", reason: says(errDeploymentFileRemoved)},
	{name: "template", under: []string{"deployment inspect"}, reason: says(errInspectTemplateRemoved)},
	{name: "login-link", reason: says(errLoginLinkRemoved)},
	// `astro api airflow` names a Deployment and an Airflow the way every
	// other command does. They were persistent flags in 1.x, so ls and
	// describe took them too.
	{name: "api-url", reason: says(errAPIURLFlagRemoved)},
	{name: "deployment-id", under: []string{"api airflow"}, needs: has("deployment"), reason: says(errDeploymentIDAPIFlag)},
	{name: "workspace-id", under: []string{"env list"}, v2Only: true, reason: says(errEnvListWorkspaceIDRemoved)},
	{name: "deployment-id", under: []string{"env list"}, v2Only: true, reason: says(errEnvListDeploymentIDRemoved)},
	// --workspace is no 1.x flag there (1.x's --workspace-id still is, as a
	// no-op), so these answer for the long name alone. needs keeps them to
	// the commands that name their Deployment with --deployment: Astro's,
	// and APC's team and user, which act on the Deployment named too.
	{name: "workspace", under: []string{"deployment team", "deployment user", "deployment token"}, needs: has("deployment"), v2Only: true, reason: says(errDeploymentByIDWorkspaceRemoved)},
	// 1.x's list commands shared --json, --template and -o
	// table|json|template (pkg/output's AddFlags), and api ls and describe
	// had --json.
	{name: "json", needs: cliout.HasOutput, reason: says(errJSONFlagRemoved)},
	{name: "template", needs: cliout.HasOutput, reason: says(errTemplateFlagRemoved)},
	// The env commands' --format took table, json and yaml (and dotenv on
	// env variable): -o takes json and dotenv where they are offered.
	{name: "format", needs: cliout.HasOutput, reason: formatRemoved},
	// --force skipped the confirmation that --yes skips now.
	{name: "force", needs: has("yes"), reason: forceRemoved},
	// astro deploy's 1.x-project flags. Astro's deploy and APC's both have
	// --deployment for --save to point at.
	{name: "save", under: []string{"deploy"}, needs: has("deployment"), reason: says(errDeploySaveRemoved)},
	{name: "pytest", under: []string{"deploy"}, reason: says(errDeployPytestRemoved)},
	{name: "test", under: []string{"deploy"}, reason: says(errDeployTestRemoved)},
	{name: "env", under: []string{"deploy"}, reason: says(errDeployEnvRemoved)},
	{name: "parse", under: []string{"deploy"}, reason: says(errDeployParseRemoved)},
	{name: "dags-path", under: []string{"deploy"}, reason: says(errDeployDagsPathRemoved)},
	{name: "dag-bundle-name", under: []string{"deploy"}, reason: says(errDeployBundleNameRemoved)},
	{name: "deployment-name", under: []string{"deploy"}, reason: says(errDeployNameRemoved)},
	{name: "no-cache", under: []string{"deploy"}, reason: says(errAPCDeployNoCache)},
	{name: "non-dags", under: []string{"deploy"}, reason: says(errDeployNonDagsRemoved)},
	{name: "non-dags-mount-path", under: []string{"deploy"}, reason: says(errDeployNonDagsMountPathRemoved)},
	{name: "non-dags-local-path", under: []string{"deploy"}, reason: says(errDeployNonDagsLocalPathRemoved)},
	{name: "non-dags-bundle-type", under: []string{"deploy"}, reason: says(errDeployNonDagsBundleTypeRemoved)},
	{name: "build-secrets", needs: has("build-secret"), reason: says(errBuildSecretsRemoved)},
	// The flags 1.x kept hidden and deprecated.
	{name: "enforce-cicd", needs: has("cicd-enforcement"), reason: says(errEnforceCICDRemoved)},
	{name: "cluster-type", under: []string{"deployment create"}, needs: has("type"), reason: says(errClusterTypeRemoved)},
	{name: "force-upgrade-to-af3", under: []string{"deploy"}, reason: says(errForceUpgradeToAF3Remove)},
	{name: "hard", under: []string{"deployment delete"}, reason: says(errHardDeleteRemoved)},

	// The astro dev commands' flags, on the commands that replaced them
	// (v1Predecessors).
	{name: "force", under: initPaths, reason: says(errInitForceRemoved)},
	{name: "from-template", under: initPaths, reason: says(errInitTemplateRemoved)},
	{name: "runtime-version", under: initPaths, reason: says(errInitRuntimeRemoved)},
	{name: "remote-execution-enabled", under: initPaths, reason: says(errInitRemoteExecRemoved)},
	{name: "remote-image-repository", under: initPaths, reason: says(errInitRemoteRepoRemoved)},
	{name: "use-astronomer-certified", under: initPaths, reason: says(errInitACRemoved)},
	{name: "docker", under: initPaths, reason: says(errInitDockerRemoved)},
	{name: "standalone", under: initPaths, reason: says(errInitStandaloneRemoved)},

	{name: "standalone", under: startPaths, reason: says(errStartStandaloneRemoved)},
	{name: "docker", under: restartPaths, reason: restartModeRemoved},
	{name: "standalone", under: restartPaths, reason: restartModeRemoved},
	{name: "port", under: restartPaths, reason: says(errRestartPortRemoved)},
	{name: "kill", under: restartPaths, reason: says(errRestartKillRemoved)},
	{name: "compose-file", under: startLikePaths, reason: says(errStartComposeFileRemoved)},
	{name: "deployment-id", under: startLikePaths, reason: says(errStartDeploymentIDRemoved)},
	{name: "workspace-id", under: startLikePaths, reason: says(errStartWorkspaceIDRemoved)},
	{name: "foreground", under: startLikePaths, reason: foregroundRemoved},
	{name: "image-name", under: startLikePaths, reason: says(errStartImageNameRemoved)},
	{name: "no-browser", under: startLikePaths, reason: says(errStartNoBrowserRemoved)},
	{name: "no-proxy", under: startLikePaths, reason: says(errStartNoProxyRemoved)},
	{name: "settings-file", under: startLikePaths, reason: says(errStartSettingsRemoved)},
	{name: "wait", under: startLikePaths, reason: waitRemoved},
	{name: "env", under: append(slices.Clone(startLikePaths), checkPaths...), reason: envFileRemoved},
	{name: "no-cache", under: startLikePaths, reason: startNoCacheRemoved},
	{name: "no-cache", under: packagePaths, reason: says("astro package " + buildsOnTheEngineCache)},

	// astro local check builds no image, in either mode.
	{name: "docker", under: checkPaths, reason: checkBuildsNoImage},
	{name: "standalone", under: checkPaths, reason: checkBuildsNoImage},
	{name: "image-name", under: checkPaths, reason: checkBuildsNoImage},
	{name: "build-secret", under: checkPaths, reason: checkBuildsNoImage},
	{name: "build-secrets", under: checkPaths, reason: checkBuildsNoImage},

	{name: "docker", under: packagePaths, reason: packageEitherMode},
	{name: "standalone", under: packagePaths, reason: packageEitherMode},
	{name: "image-name", under: packagePaths, reason: says(errPackageImageNameRemoved)},

	// The commands that act on whatever is running, or was.
	{name: "docker", under: eitherModePaths, reason: eitherModeRemoved},
	{name: "standalone", under: eitherModePaths, reason: eitherModeRemoved},

	// astro dev logs picked a component with a flag of its own, and astro
	// dev bash a container.
	{name: "api-server", under: logsPaths, reason: componentFlagRemoved},
	{name: "dag-processor", under: logsPaths, reason: componentFlagRemoved},
	{name: "scheduler", under: logsPaths, reason: componentFlagRemoved},
	{name: "triggerer", under: logsPaths, reason: componentFlagRemoved},
	{name: "webserver", under: logsPaths, reason: componentFlagRemoved},
	{name: "api-server", under: shellPaths, reason: shellComponentRemoved},
	{name: "dag-processor", under: shellPaths, reason: shellComponentRemoved},
	{name: "postgres", under: shellPaths, reason: shellComponentRemoved},
	{name: "scheduler", under: shellPaths, reason: shellComponentRemoved},
	{name: "triggerer", under: shellPaths, reason: shellComponentRemoved},
	{name: "webserver", under: shellPaths, reason: shellComponentRemoved},
}

// The v2 commands that replaced astro dev commands, as under lists them.
var (
	initPaths       = []string{"init"}
	startPaths      = []string{"local start", "start"}
	restartPaths    = []string{"local restart"}
	startLikePaths  = append(slices.Clone(startPaths), restartPaths...)
	logsPaths       = []string{"local logs", "logs"}
	shellPaths      = []string{"local shell"}
	checkPaths      = []string{"local check"}
	packagePaths    = []string{"package"}
	eitherModePaths = []string{"local stop", "stop", "local status", "local logs", "logs", "local shell", "local reset"}
)

// v1Predecessors maps a v2 command that replaced a 1.x command of another
// name, without keeping that name as an alias, to the 1.x command's path, so
// the flags 1.x had there are reported on it as a renamed command's are:
// `astro init --runtime-version` says what replaced it. They are the astro dev
// commands scaffold.DevReplacements maps to an astro command, and the root's
// start, stop and logs, which are astro local's
// (TestV1PredecessorsFollowTheDevMapping keeps the two in step). Left out:
//
//   - astro dev run, which parsed no flags of its own in 1.x;
//   - astro dev pytest, whose replacement is uv run pytest, not an astro
//     command;
//   - astro dev object export and import, whose flags picked what to move
//     between airflow_settings.yaml and a running Airflow, which no flag of
//     astro local env does: the astro dev stub's guidance is the answer there.
var v1Predecessors = map[string]string{
	"init":          "dev init",
	"local start":   "dev start",
	"start":         "dev start",
	"local restart": "dev restart",
	"local stop":    "dev stop",
	"stop":          "dev stop",
	"local status":  "dev ps",
	"local logs":    "dev logs",
	"logs":          "dev logs",
	"local shell":   "dev bash",
	"local check":   "dev parse",
	"package":       "dev build",
	"local reset":   "dev kill",
}

func restartModeRemoved(_ *cobra.Command, name string) string {
	start := "astro local start"
	if name == "docker" {
		start += " --docker"
	}
	return "astro local restart keeps the mode Airflow is running in. To change it, use astro local stop, then " + start
}

func foregroundRemoved(cmd *cobra.Command, _ string) string {
	return cmd.CommandPath() + " returns once Airflow is up, and leaves it running. " +
		"Follow its logs with astro local logs -f, and stop it with astro local stop"
}

func waitRemoved(cmd *cobra.Command, _ string) string {
	return "set the ASTRO_LOCAL_HEALTH_TIMEOUT environment variable to a Go duration instead, as in " +
		"ASTRO_LOCAL_HEALTH_TIMEOUT=10m " + cmd.CommandPath()
}

func envFileRemoved(cmd *cobra.Command, _ string) string {
	return cmd.CommandPath() + " reads the .env at the project root. " +
		"To set a value there, use astro local env variable set <NAME> --plain"
}

// startNoCacheRemoved says what a start builds in each mode: standalone
// builds no image, and syncs the project's environment with uv instead.
// --docker is named where cmd has it; a restart keeps the mode it finds.
func startNoCacheRemoved(cmd *cobra.Command, _ string) string {
	docker := "Docker mode"
	if cmd.Flag("docker") != nil {
		docker += " (--docker)"
	}
	return "standalone mode builds no image, and " + docker + " " + buildsOnTheEngineCache
}

func checkBuildsNoImage(_ *cobra.Command, _ string) string {
	return "astro local check parses the Dags in the project's own environment and builds no image, so drop it"
}

func packageEitherMode(_ *cobra.Command, _ string) string {
	return "astro package builds the same image whichever mode Airflow runs in locally, so drop it"
}

func eitherModeRemoved(cmd *cobra.Command, _ string) string {
	return cmd.CommandPath() + " works in either mode, so drop it"
}

func componentFlagRemoved(_ *cobra.Command, name string) string {
	return "use --component " + name
}

func shellComponentRemoved(_ *cobra.Command, name string) string {
	const where = "astro local shell opens its shell in the scheduler's container in Docker mode, and in the project's environment in standalone mode"
	if name == "scheduler" {
		return where + ", so drop it"
	}
	return where + ", and has no flag for another component"
}

// removedFlagErr is a run refused for passing a removed 1.x flag.
type removedFlagErr struct{ msg string }

func (e *removedFlagErr) Error() string { return e.msg }

// removedFlagError is what a run is told when it passed a removed 1.x flag
// that cmd does not have, or nil when err is not about one. It runs from the
// root's flag error func (flagError), while cobra parses flags, so it fails
// before any pre-run logs in or asks an API anything, and as a usage error
// (exit 2), which Execute reports under --output json as the error object.
func removedFlagError(cmd *cobra.Command, err error) error {
	typed, isShorthand, spelling := unknownFlag(err)
	if spelling == "" {
		return nil
	}
	v1 := v1FlagOn(cmd, typed, isShorthand)
	if v1 == nil {
		// A command 1.x did not have: only an entry made for it applies.
		if f := findRemovedFlagIn(cmd, typed, true); f != nil {
			return cliout.Usage(&removedFlagErr{f.message(cmd, spelling)})
		}
		return nil
	}
	// The flag is still there, without the letter 1.x gave it (astro init
	// -n, astro local start -p).
	if isShorthand && cmd.Flag(v1.name) != nil {
		return cliout.Usage(&removedFlagErr{shorthandRemoved(spelling, v1.name)})
	}
	f := findRemovedFlag(cmd, v1.name)
	if f == nil {
		return nil
	}
	return cliout.Usage(&removedFlagErr{f.message(cmd, spelling)})
}

// shorthandRemoved is what a run is told for a shorthand 1.x had, of a flag
// the command still has under its long name alone.
func shorthandRemoved(spelling, name string) string {
	return removedMessage(spelling, "use --"+name)
}

// findRemovedFlag returns the entry for the flag called name that applies on
// cmd, a flag 1.x had there, or nil when none does.
func findRemovedFlag(cmd *cobra.Command, name string) *removedFlag {
	return findRemovedFlagIn(cmd, name, false)
}

// findRemovedFlagIn returns the entry for the flag called name that applies on
// cmd among the v2Only entries, or among the others, so neither kind can
// shadow the other whatever their order.
func findRemovedFlagIn(cmd *cobra.Command, name string, v2Only bool) *removedFlag {
	// `astro local stop -f`: stop still has --force, without the -f, so
	// "--force was removed" would be false there.
	if cmd.Flag(name) != nil {
		return nil
	}
	for i := range removedFlags {
		f := &removedFlags[i]
		if f.name != name || f.v2Only != v2Only {
			continue
		}
		if len(f.under) > 0 && !slices.ContainsFunc(f.under, func(path string) bool { return isUnder(cmd, path) }) {
			continue
		}
		if f.needs != nil && !f.needs(cmd) {
			continue
		}
		return f
	}
	return nil
}

// isUnder reports whether cmd is the command at path below its root, or one
// of that command's subcommands, by cobra's own path for cmd.
func isUnder(cmd *cobra.Command, path string) bool {
	own := pathBelowRoot(cmd)
	return own == path || strings.HasPrefix(own, path+" ")
}

// pathBelowRoot is cmd's path without the root's name: "deployment list".
func pathBelowRoot(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
}

func has(flag string) func(*cobra.Command) bool {
	return func(cmd *cobra.Command) bool { return cmd.Flag(flag) != nil }
}

// forceRemoved names --yes with its shorthand when cmd gives it one, and
// says so differently when the run passed it already (`-yf`).
func forceRemoved(cmd *cobra.Command, _ string) string {
	yes := cmd.Flag("yes")
	spelled := "--yes"
	if yes.Shorthand != "" {
		spelled += " (-" + yes.Shorthand + ")"
	}
	// Changed alone would include --yes=false, which skips nothing.
	if yes.Changed && yes.Value.String() == "true" {
		return spelled + ", which you passed, already skips the confirmation, so drop it"
	}
	return "use " + spelled
}

// formatRemoved names the formats cmd's -o takes, which differ by command
// (dotenv on `env variable get` and `list`), and says what became of yaml,
// which 1.x's --format took everywhere.
func formatRemoved(cmd *cobra.Command, _ string) string {
	msg := "use -o (--output), which takes " + cliout.FormatList(cmd)
	if !cliout.Offers(cmd, "yaml") {
		msg += "; there is no yaml, so use -o json"
	}
	return msg
}

// The trees v1_flags.tsv records, and the annotation on a root naming the one
// it is the v2 side of (markV1Tree).
const (
	v1TreeAstro       = "astro"
	v1TreeAstroHosted = "astro-hosted"
	v1TreeAPC         = "apc"
	v1TreeAnnotation  = "v1-tree"
)

// markV1Tree records on root which platform's 1.x tree a script on this
// machine was written against, APC's or Astro's. Whether Astro's is the one
// for a hosted organization is left to v1TreeOf, so that building a root reads
// no context: only a run that passes an unknown flag, or asks for
// completions, needs to know.
func markV1Tree(root *cobra.Command, platform string) {
	tree := v1TreeAstro
	if platform == apcPlatform {
		tree = v1TreeAPC
	}
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations[v1TreeAnnotation] = tree
}

// v1TreeOf is the 1.x tree a script was written against on the machine root
// was built on: APC's, or Astro's for a hosted organization or not, decided
// as cmd/astro decides the hosted-only flags it registers. A run reads it at
// most a few times, when a flag fails to parse or a shell asks for
// completions, and the context does not change in between.
func v1TreeOf(root *cobra.Command) string {
	tree := root.Annotations[v1TreeAnnotation]
	if tree == v1TreeAstro && organization.IsOrgHosted() {
		return v1TreeAstroHosted
	}
	return tree
}

// v1FlagsFile is every flag 1.x had, on every command, in each tree.
//
//go:embed v1_flags.tsv
var v1FlagsFile string

// v1Flag is a line of v1_flags.tsv.
type v1Flag struct {
	trees                 []string
	path, name, shorthand string
	isBool                bool
}

// v1Flags is v1_flags.tsv, read the first time a run needs it: a run that
// passes no unknown flag never does. A file that does not parse is
// TestV1FlagsFileIsWellFormed's to catch; a run given one reports every flag
// it does not know as cobra does, rather than failing over a tombstone.
var v1Flags = sync.OnceValue(func() []v1Flag { return loadV1Flags(v1FlagsFile) })

// loadV1Flags is the flags in file, or none when it does not parse.
func loadV1Flags(file string) []v1Flag {
	flags, err := parseV1Flags(file)
	if err != nil {
		return nil
	}
	return flags
}

// parseV1Flags reads v1_flags.tsv. It takes CRLF line ends as well as LF:
// .gitattributes pins the file to LF, but a checkout that predates that, or
// a tool that rewrites it, embeds whatever is on disk.
func parseV1Flags(file string) ([]v1Flag, error) {
	var flags []v1Flag
	for line := range strings.Lines(file) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 5 || (f[4] != "bool" && f[4] != "string") {
			return nil, fmt.Errorf("v1_flags.tsv: malformed line %q", line)
		}
		flags = append(flags, v1Flag{trees: strings.Split(f[0], ","), path: f[1], name: f[2], shorthand: f[3], isBool: f[4] == "bool"})
	}
	return flags, nil
}

// v1FlagsOn is every flag 1.x had on the command at cmd's path, or on the
// 1.x command it replaced (v1Predecessors), in the tree cmd's root was built
// for.
func v1FlagsOn(cmd *cobra.Command) []v1Flag {
	paths := pathSpellings(cmd)
	if was, ok := v1Predecessors[pathBelowRoot(cmd)]; ok && cmd.HasParent() {
		paths = append(paths, was)
	}
	return v1FlagsAt(v1Flags(), v1TreeOf(cmd.Root()), paths)
}

// v1FlagsAt is every flag in flags that tree had on a command at one of paths.
func v1FlagsAt(flags []v1Flag, tree string, paths []string) []v1Flag {
	var on []v1Flag
	for _, f := range flags {
		if slices.Contains(f.trees, tree) && slices.Contains(paths, f.path) {
			on = append(on, f)
		}
	}
	return on
}

// pathSpellings is every way to type cmd's path below the root, each word
// its command's name or one of its aliases, the names alone first: a command
// v2 renamed and kept the 1.x name of as an alias is at the 1.x path under
// one of these.
func pathSpellings(cmd *cobra.Command) []string {
	if !cmd.HasParent() {
		return nil
	}
	names := append([]string{cmd.Name()}, cmd.Aliases...)
	if !cmd.Parent().HasParent() {
		return names
	}
	var out []string
	for _, parent := range pathSpellings(cmd.Parent()) {
		for _, name := range names {
			out = append(out, parent+" "+name)
		}
	}
	return out
}

// v1FlagOn is the flag 1.x had on cmd typed as typed (a shorthand's letter
// when isShorthand), or nil when it had none.
func v1FlagOn(cmd *cobra.Command, typed string, isShorthand bool) *v1Flag {
	return findV1Flag(v1FlagsOn(cmd), typed, isShorthand)
}

func findV1Flag(flags []v1Flag, typed string, isShorthand bool) *v1Flag {
	for i := range flags {
		if f := &flags[i]; (isShorthand && f.shorthand == typed) || (!isShorthand && f.name == typed) {
			return f
		}
	}
	return nil
}

// isRemovedFlagErr reports whether err is a run refused for a removed flag.
func isRemovedFlagErr(err error) bool {
	var removed *removedFlagErr
	return errors.As(err, &removed)
}

// acceptRemovedFlags keeps a removed 1.x flag from standing in the way of a
// run that asks for help or for shell completions, which cobra answers only
// once the flags parse.
//
// A run that asks for help gets the help instead of the refusal: one that
// passes -h or --help (or --help=true) as a word of its own before any "--",
// wherever it stands (hasHelpFlag), or that pflag has already read help from
// when it reaches the removed flag (`-hf`). It does not matter that such a -h
// might really be a flag's value (`--name -h`): the worst that does is show
// help instead of an error. A group that reaches the removed letter first
// (`-fh`) is refused, as pflag stops there.
//
// A completion request (cobra's __complete) is never refused at all: cobra
// parses the command's flags itself there, without the flag error func, and
// fails the completion on any flag it does not know. So the command it
// completes gets each removed flag 1.x had on it as a hidden flag of the
// same arity, and each shorthand 1.x gave a flag it still has under its long
// name alone as a hidden flag of its own, which offer nothing and run
// nothing, since a completion request never runs the command
// (acceptForCompletion).
func acceptRemovedFlags(root *cobra.Command, args []string) {
	if isShellCompletion(args) {
		if len(args) > 1 {
			acceptForCompletion(root, commandWords(args))
		}
		return
	}
	askedForHelp := hasHelpFlag(args)
	refuse := root.FlagErrorFunc()
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		err = refuse(cmd, err)
		if !isRemovedFlagErr(err) {
			return err
		}
		// The value, not Changed: --help=false asks for nothing.
		if help := cmd.Flags().Lookup("help"); askedForHelp || (help != nil && help.Value.String() == "true") {
			return pflag.ErrHelp
		}
		return err
	})
}

// acceptForCompletion readies the tree for a completion request whose words
// before the one being completed are args.
//
// The command completed is the one cobra finds for args, and only it gets
// the removed flags 1.x had on it, so a removed flag typed after the
// command's name completes. One typed before a subcommand's name
// (`deployment --json list`) is left to cobra, as a run of that line is: the
// parent never had it, so cobra reads it as an unknown flag there. And only
// when a flag typed is one the command does not have, so a request that
// passes no unknown flag (nearly all) never reads the inventory.
func acceptForCompletion(root *cobra.Command, args []string) {
	find := root.Find
	if root.TraverseChildren {
		find = root.Traverse
	}
	cmd, _, err := find(args)
	if err != nil || cmd.DisableFlagParsing || !typesUnknownFlag(cmd, args) {
		return
	}
	on := v1FlagsOn(cmd)
	for i := range on {
		switch f := &on[i]; {
		case findRemovedFlag(cmd, f.name) != nil:
			addHiddenFlag(cmd, f)
		case f.shorthand != "" && cmd.Flag(f.name) != nil:
			addHiddenShorthand(cmd, f)
		}
	}
}

// typesUnknownFlag reports whether args, before any "--", pass a flag cmd
// does not have: a long flag, or a letter of a shorthand group that pflag
// would read as a flag rather than as the value of the one before it.
func typesUnknownFlag(cmd *cobra.Command, args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			continue
		}
		if name, ok := strings.CutPrefix(arg, "--"); ok {
			name, _, _ = strings.Cut(name, "=")
			if cmd.Flag(name) == nil {
				return true
			}
			continue
		}
		letters, _, _ := strings.Cut(arg[1:], "=")
		for i := range len(letters) {
			flag := shorthandFlag(cmd, letters[i:i+1])
			if flag == nil {
				return true
			}
			if flag.NoOptDefVal == "" {
				break // the rest of the group is its value
			}
		}
	}
	return false
}

// addHiddenFlag gives cmd 1.x's flag f as a hidden flag of the same arity,
// for a completion request to parse, unless cmd has a flag of that name. It
// drops a shorthand cmd uses already, as pflag panics on a second one.
func addHiddenFlag(cmd *cobra.Command, f *v1Flag) {
	if cmd.Flag(f.name) != nil {
		return
	}
	shorthand := f.shorthand
	if shorthand == "h" || shorthandFlag(cmd, shorthand) != nil {
		shorthand = ""
	}
	if f.isBool {
		cmd.Flags().BoolP(f.name, shorthand, false, "")
	} else {
		cmd.Flags().StringP(f.name, shorthand, "", "")
	}
	_ = cmd.Flags().MarkHidden(f.name) //nolint:errcheck // it was registered on the line above
}

// addHiddenShorthand gives cmd the shorthand 1.x gave its flag f, which cmd
// still has under its long name alone (`astro init -n`), as a hidden flag of
// f's arity named for the letter, for a completion request to parse, unless
// cmd uses the letter already.
func addHiddenShorthand(cmd *cobra.Command, f *v1Flag) {
	if f.shorthand == "h" || shorthandFlag(cmd, f.shorthand) != nil {
		return
	}
	name := "v1-shorthand-" + f.shorthand
	if f.isBool {
		cmd.Flags().BoolP(name, f.shorthand, false, "")
	} else {
		cmd.Flags().StringP(name, f.shorthand, "", "")
	}
	_ = cmd.Flags().MarkHidden(name) //nolint:errcheck // it was registered on the line above
}

func shorthandFlag(cmd *cobra.Command, letter string) *pflag.Flag {
	if f := cmd.LocalFlags().ShorthandLookup(letter); f != nil {
		return f
	}
	return cmd.InheritedFlags().ShorthandLookup(letter)
}
