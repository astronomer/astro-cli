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

// What a run passing a removed 1.x flag is told. Each one starts with the
// flag's own spelling and "was removed in Astro CLI v2", which
// TestRemovedFlagsSayWhatReplacedThem holds them to.
const (
	errJSONFlagRemoved     = "--json was removed in Astro CLI v2: use -o json"
	errTemplateFlagRemoved = "--template was removed in Astro CLI v2: use -o json, and a JSON tool such as jq to pick fields"
	errAPIURLFlagRemoved   = "--api-url was removed in Astro CLI v2: use --url"
	errDeploymentIDAPIFlag = "--deployment-id was removed in Astro CLI v2: use --deployment (-d), which takes a Deployment id or a link name from pyproject.toml"

	errDeploymentFileRemoved = "--deployment-file was removed in Astro CLI v2. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL + ". " +
		"To copy a Deployment, use astro deployment create --clone <deployment> --name <new name>"
	errInspectTemplateRemoved = "--template was removed in Astro CLI v2. " +
		"To copy a Deployment, use astro deployment create --clone <deployment> --name <new name>. " +
		"To manage Deployments as code, use the Astro Terraform provider: " + terraformProviderURL
	// A switch has not logged in again since 2023, so there is no login to
	// link to.
	errLoginLinkRemoved = "--login-link was removed in Astro CLI v2: switching organizations no longer re-authenticates. " +
		"To log in on another device, use astro login --login-link"

	// astro deploy ships only pyproject.toml projects in v2, so the flags only
	// the 1.x project's deploy read went with it. A 1.x project deploys with
	// Astro CLI 1.x, flags and all.
	errDeploySaveRemoved = "--save was removed in Astro CLI v2: astro deploy asks which Deployment to deploy to unless you name one, as the argument or with --deployment. " +
		"To preselect one in that list, link it with astro link add and mark it default = true under [tool.astro.deployments]"
	errDeployPytestRemoved     = "--pytest was removed in Astro CLI v2: run your tests before deploying, with uv run pytest && astro deploy"
	errDeployTestRemoved       = "--test was removed in Astro CLI v2: run your tests before deploying, with uv run pytest <path> && astro deploy"
	errDeployEnvRemoved        = "--env was removed in Astro CLI v2: astro deploy runs no tests, so it reads no test env file. Run your tests with uv run pytest before deploying"
	errDeployParseRemoved      = "--parse was removed in Astro CLI v2: check your Dags before deploying, with astro local check && astro deploy"
	errDeployDagsPathRemoved   = "--dags-path was removed in Astro CLI v2: astro deploy --dags ships the project's dags directory, so run it from the project"
	errDeployBundleNameRemoved = "--dag-bundle-name was removed in Astro CLI v2: deploying to a named Dag bundle is not supported yet. Use Astro CLI 1.x for it"
	errDeployNameRemoved       = "--deployment-name was removed in Astro CLI v2: name the Deployment as the argument or with --deployment, which take a link name or a Deployment id"
	// astro deploy --non-dags became astro deploy bundle, a subcommand of
	// its own with the bundle's directory as its argument.
	errDeployNonDagsRemoved = "--non-dags was removed in Astro CLI v2: use astro deploy bundle [PATH] --deployment <Deployment id, name or link name>, " +
		"which deploys a non-Dag bundle such as a dbt project from its directory"
	errDeployNonDagsMountPathRemoved  = "--non-dags-mount-path was removed in Astro CLI v2: use astro deploy bundle --mount-path"
	errDeployNonDagsLocalPathRemoved  = "--non-dags-local-path was removed in Astro CLI v2: pass the bundle's directory as the argument, astro deploy bundle <PATH>"
	errDeployNonDagsBundleTypeRemoved = "--non-dags-bundle-type was removed in Astro CLI v2: use astro deploy bundle --type, which detects a dbt project by itself"
	// 1.x kept --build-secrets, hidden and deprecated, as an alias of the
	// repeatable --build-secret that replaced it.
	errBuildSecretsRemoved = "--build-secrets was removed in Astro CLI v2: use --build-secret, once for each secret"
	errAPCDeployNoCache    = "--no-cache was removed in Astro CLI v2: astro deploy builds on the container engine's cache, which a changed dependency or file invalidates. " +
		"To build from nothing, clear the engine's build cache first, with docker builder prune"

	// The flags 1.x marked deprecated, hidden but accepted, which v2 dropped.
	errEnforceCICDRemoved      = "--enforce-cicd was removed in Astro CLI v2: use --cicd-enforcement enable, or --cicd-enforcement disable"
	errClusterTypeRemoved      = "--cluster-type was removed in Astro CLI v2: use --type, which takes the same standard or dedicated"
	errForceUpgradeToAF3Remove = "--force-upgrade-to-af3 was removed in Astro CLI v2: a deploy that moves a Deployment from Airflow 2 to Airflow 3 needs no flag, " +
		"as it needed none in 1.x, which ignored this one, so drop it"
	errHardDeleteRemoved = "--hard was removed in Astro CLI v2: deleting a Deployment always removes all of its infrastructure and records, " +
		"as it did in 1.x, which ignored --hard, so drop it"

	// astro dev init's flags, on astro init.
	errInitForceRemoved    = "--force was removed in Astro CLI v2: astro init asks nothing before it writes, in an empty directory or not, so drop it"
	errInitTemplateRemoved = "--from-template was removed in Astro CLI v2: there are no templates to choose from, and astro init writes one starter project, with an example Dag in dags/"
	errInitRuntimeRemoved  = "--runtime-version was removed in Astro CLI v2: a project pins Airflow, with --airflow-version or the apache-airflow requirement in pyproject.toml, " +
		"and Docker mode builds on the newest Astro Runtime for it. To pin one Runtime build, set runtime = \"<version>\" under [tool.astro] in pyproject.toml"
	errInitRemoteExecRemoved = "--remote-execution-enabled was removed in Astro CLI v2: astro init writes no client image files. " +
		"To deploy a client image for remote execution, add a Dockerfile.client to the project and use astro remote deploy"
	errInitRemoteRepoRemoved = "--remote-image-repository was removed in Astro CLI v2: set the repository astro remote deploy pushes the client image to with " +
		"astro config set deploy.remote_execution.client_registry <registry>/<repository>"
	errInitACRemoved = "--use-astronomer-certified was removed in Astro CLI v2: a project runs Apache Airflow, on Astro Runtime in Docker mode, " +
		"and there is no Astronomer Certified image to choose. Pin Airflow with --airflow-version"
	errInitDockerRemoved     = "--docker was removed in Astro CLI v2: astro init makes one project for both modes. Choose Docker when you start it, with astro local start --docker"
	errInitStandaloneRemoved = "--standalone was removed in Astro CLI v2: astro init makes one project for both modes, and astro local start runs it standalone by default, so drop it"

	// astro dev start's and restart's flags, on astro local start and
	// restart (and the root's astro start).
	errStartStandaloneRemoved  = "--standalone was removed in Astro CLI v2: standalone is the default mode, so drop it"
	errStartComposeFileRemoved = "--compose-file was removed in Astro CLI v2: Docker mode writes its own compose file, and merges a docker-compose.override.yml " +
		"at the project root over it. Put your changes there"
	errStartDeploymentIDRemoved = "--deployment-id was removed in Astro CLI v2: a start reads no Deployment's values. " +
		"It reads the Environment Manager of the workspace pyproject.toml names, with workspace = \"<workspace id>\" under [tool.astro]"
	errStartWorkspaceIDRemoved = "--workspace-id was removed in Astro CLI v2: a start reads the Environment Manager of the workspace pyproject.toml names, " +
		"with workspace = \"<workspace id>\" under [tool.astro]"
	errStartImageNameRemoved = "--image-name was removed in Astro CLI v2: Docker mode builds the project's image itself, from pyproject.toml, " +
		"or from the Dockerfile that dockerfile = \"<path>\" under [tool.astro] names"
	errStartNoBrowserRemoved = "--no-browser was removed in Astro CLI v2: a start opens no browser, so drop it. To open the Airflow UI, use astro local open"
	errStartNoProxyRemoved   = "--no-proxy was removed in Astro CLI v2: Airflow always answers on its own port on localhost too, the URL astro local status shows as direct. " +
		"To choose that port, use astro local start --port"
	errStartSettingsRemoved = "--settings-file was removed in Astro CLI v2: a start sets the connections, variables and pools pyproject.toml declares, " +
		"in [tool.astro.env] and [tool.astro.pools]. astro init carries an airflow_settings.yaml into them"
	errRestartPortRemoved = "--port was removed in Astro CLI v2: astro local restart keeps the port Airflow is running on. " +
		"To change it, use astro local stop, then astro local start --port <port>"
	errRestartKillRemoved = "--kill was removed in Astro CLI v2: to wipe the local database and logs first, use astro local reset --yes, then astro local start"

	// astro dev build's, on astro package.
	errPackageImageNameRemoved = "--image-name was removed in Astro CLI v2: astro package always builds the image, from pyproject.toml, " +
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
	// msg is the message; msgFor, when set, builds it instead, for one that
	// depends on cmd or on how the flag was typed ("-f" or "--force").
	msg    string
	msgFor func(cmd *cobra.Command, typed string) string
	// named, when set, builds it from cmd and the flag's name instead, for
	// one message several entries share.
	named func(cmd *cobra.Command, name string) string
}

func (f *removedFlag) message(cmd *cobra.Command, typed string) string {
	switch {
	case f.msgFor != nil:
		return f.msgFor(cmd, typed)
	case f.named != nil:
		return f.named(cmd, f.name)
	}
	return f.msg
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
// first entry that applies wins.
//
// A future removal belongs here too: TestEveryV1FlagStillWorksOrSaysWhatReplacedIt
// fails on a flag in v1_flags.tsv that the v2 tree neither has nor reports
// here, and on an entry no flag in it reaches.
var removedFlags = []removedFlag{
	// Deployments as code moved to Terraform.
	{name: "deployment-file", msg: errDeploymentFileRemoved},
	{name: "template", under: []string{"deployment inspect"}, msg: errInspectTemplateRemoved},
	{name: "login-link", msg: errLoginLinkRemoved},
	// `astro api airflow` names a Deployment and an Airflow the way every
	// other command does. They were persistent flags in 1.x, so ls and
	// describe took them too.
	{name: "api-url", msg: errAPIURLFlagRemoved},
	{name: "deployment-id", under: []string{"api airflow"}, needs: has("deployment"), msg: errDeploymentIDAPIFlag},
	// 1.x's list commands shared --json, --template and -o
	// table|json|template (pkg/output's AddFlags), and api ls and describe
	// had --json.
	{name: "json", needs: cliout.HasOutput, msg: errJSONFlagRemoved},
	{name: "template", needs: cliout.HasOutput, msg: errTemplateFlagRemoved},
	// The env commands' --format took table, json and yaml (and dotenv on
	// env variable): -o takes json and dotenv where they are offered.
	{name: "format", needs: cliout.HasOutput, msgFor: formatRemoved},
	// --force skipped the confirmation that --yes skips now.
	{name: "force", needs: has("yes"), msgFor: forceRemoved},
	// astro deploy's 1.x-project flags. Astro's deploy has --deployment for
	// --save to point at; APC's names its Deployment by the argument alone.
	{name: "save", under: []string{"deploy"}, needs: has("deployment"), msg: errDeploySaveRemoved},
	{name: "pytest", under: []string{"deploy"}, msg: errDeployPytestRemoved},
	{name: "test", under: []string{"deploy"}, msg: errDeployTestRemoved},
	{name: "env", under: []string{"deploy"}, msg: errDeployEnvRemoved},
	{name: "parse", under: []string{"deploy"}, msg: errDeployParseRemoved},
	{name: "dags-path", under: []string{"deploy"}, msg: errDeployDagsPathRemoved},
	{name: "dag-bundle-name", under: []string{"deploy"}, msg: errDeployBundleNameRemoved},
	{name: "deployment-name", under: []string{"deploy"}, msg: errDeployNameRemoved},
	{name: "no-cache", under: []string{"deploy"}, msg: errAPCDeployNoCache},
	{name: "non-dags", under: []string{"deploy"}, msg: errDeployNonDagsRemoved},
	{name: "non-dags-mount-path", under: []string{"deploy"}, msg: errDeployNonDagsMountPathRemoved},
	{name: "non-dags-local-path", under: []string{"deploy"}, msg: errDeployNonDagsLocalPathRemoved},
	{name: "non-dags-bundle-type", under: []string{"deploy"}, msg: errDeployNonDagsBundleTypeRemoved},
	{name: "build-secrets", needs: has("build-secret"), msg: errBuildSecretsRemoved},
	// The flags 1.x kept hidden and deprecated.
	{name: "enforce-cicd", needs: has("cicd-enforcement"), msg: errEnforceCICDRemoved},
	{name: "cluster-type", under: []string{"deployment create"}, needs: has("type"), msg: errClusterTypeRemoved},
	{name: "force-upgrade-to-af3", under: []string{"deploy"}, msg: errForceUpgradeToAF3Remove},
	{name: "hard", under: []string{"deployment delete"}, msg: errHardDeleteRemoved},

	// The astro dev commands' flags, on the commands that replaced them
	// (v1Predecessors).
	{name: "force", under: initPaths, msg: errInitForceRemoved},
	{name: "from-template", under: initPaths, msg: errInitTemplateRemoved},
	{name: "runtime-version", under: initPaths, msg: errInitRuntimeRemoved},
	{name: "remote-execution-enabled", under: initPaths, msg: errInitRemoteExecRemoved},
	{name: "remote-image-repository", under: initPaths, msg: errInitRemoteRepoRemoved},
	{name: "use-astronomer-certified", under: initPaths, msg: errInitACRemoved},
	{name: "docker", under: initPaths, msg: errInitDockerRemoved},
	{name: "standalone", under: initPaths, msg: errInitStandaloneRemoved},

	{name: "standalone", under: startPaths, msg: errStartStandaloneRemoved},
	{name: "docker", under: restartPaths, named: restartModeRemoved},
	{name: "standalone", under: restartPaths, named: restartModeRemoved},
	{name: "port", under: restartPaths, msg: errRestartPortRemoved},
	{name: "kill", under: restartPaths, msg: errRestartKillRemoved},
	{name: "compose-file", under: startLikePaths, msg: errStartComposeFileRemoved},
	{name: "deployment-id", under: startLikePaths, msg: errStartDeploymentIDRemoved},
	{name: "workspace-id", under: startLikePaths, msg: errStartWorkspaceIDRemoved},
	{name: "foreground", under: startLikePaths, msgFor: foregroundRemoved},
	{name: "image-name", under: startLikePaths, msg: errStartImageNameRemoved},
	{name: "no-browser", under: startLikePaths, msg: errStartNoBrowserRemoved},
	{name: "no-proxy", under: startLikePaths, msg: errStartNoProxyRemoved},
	{name: "settings-file", under: startLikePaths, msg: errStartSettingsRemoved},
	{name: "wait", under: startLikePaths, msgFor: waitRemoved},
	{name: "env", under: append(slices.Clone(startLikePaths), checkPaths...), msgFor: envFileRemoved},
	{name: "no-cache", under: append(slices.Clone(startLikePaths), packagePaths...), msgFor: noCacheRemoved},

	// astro local check builds no image, in either mode.
	{name: "docker", under: checkPaths, named: checkBuildsNoImage},
	{name: "standalone", under: checkPaths, named: checkBuildsNoImage},
	{name: "image-name", under: checkPaths, named: checkBuildsNoImage},
	{name: "build-secret", under: checkPaths, named: checkBuildsNoImage},
	{name: "build-secrets", under: checkPaths, named: checkBuildsNoImage},

	{name: "docker", under: packagePaths, named: packageEitherMode},
	{name: "standalone", under: packagePaths, named: packageEitherMode},
	{name: "image-name", under: packagePaths, msg: errPackageImageNameRemoved},

	// The commands that act on whatever is running, or was.
	{name: "docker", under: eitherModePaths, named: eitherModeRemoved},
	{name: "standalone", under: eitherModePaths, named: eitherModeRemoved},

	// astro dev logs picked a component with a flag of its own, and astro
	// dev bash a container.
	{name: "api-server", under: logsPaths, named: componentFlagRemoved},
	{name: "dag-processor", under: logsPaths, named: componentFlagRemoved},
	{name: "scheduler", under: logsPaths, named: componentFlagRemoved},
	{name: "triggerer", under: logsPaths, named: componentFlagRemoved},
	{name: "webserver", under: logsPaths, named: componentFlagRemoved},
	{name: "api-server", under: shellPaths, named: shellComponentRemoved},
	{name: "dag-processor", under: shellPaths, named: shellComponentRemoved},
	{name: "postgres", under: shellPaths, named: shellComponentRemoved},
	{name: "scheduler", under: shellPaths, named: shellComponentRemoved},
	{name: "triggerer", under: shellPaths, named: shellComponentRemoved},
	{name: "webserver", under: shellPaths, named: shellComponentRemoved},
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
	return "--" + name + " was removed in Astro CLI v2: astro local restart keeps the mode Airflow is running in. To change it, use astro local stop, then " + start
}

func foregroundRemoved(cmd *cobra.Command, _ string) string {
	return "--foreground was removed in Astro CLI v2: " + cmd.CommandPath() + " returns once Airflow is up, and leaves it running. " +
		"Follow its logs with astro local logs -f, and stop it with astro local stop"
}

func waitRemoved(cmd *cobra.Command, _ string) string {
	return "--wait was removed in Astro CLI v2: set the ASTRO_LOCAL_HEALTH_TIMEOUT environment variable to a Go duration instead, as in " +
		"ASTRO_LOCAL_HEALTH_TIMEOUT=10m " + cmd.CommandPath()
}

func envFileRemoved(cmd *cobra.Command, _ string) string {
	return "--env was removed in Astro CLI v2: " + cmd.CommandPath() + " reads the .env at the project root. " +
		"To set a value there, use astro local env variable set <NAME> --plain"
}

func noCacheRemoved(cmd *cobra.Command, _ string) string {
	return "--no-cache was removed in Astro CLI v2: " + cmd.CommandPath() + " builds on the container engine's cache, which a changed dependency or file invalidates. " +
		"To build from nothing, clear the engine's build cache first, with docker builder prune"
}

func checkBuildsNoImage(_ *cobra.Command, name string) string {
	return "--" + name + " was removed in Astro CLI v2: astro local check parses the Dags in the project's own environment and builds no image, so drop it"
}

func packageEitherMode(_ *cobra.Command, name string) string {
	return "--" + name + " was removed in Astro CLI v2: astro package builds the same image whichever mode Airflow runs in locally, so drop it"
}

func eitherModeRemoved(cmd *cobra.Command, name string) string {
	return "--" + name + " was removed in Astro CLI v2: " + cmd.CommandPath() + " works in either mode, so drop it"
}

func componentFlagRemoved(_ *cobra.Command, name string) string {
	return "--" + name + " was removed in Astro CLI v2: use --component " + name
}

func shellComponentRemoved(_ *cobra.Command, name string) string {
	const where = "astro local shell opens its shell in the scheduler's container in Docker mode, and in the project's environment in standalone mode"
	if name == "scheduler" {
		return "--scheduler was removed in Astro CLI v2: " + where + ", so drop it"
	}
	return "--" + name + " was removed in Astro CLI v2: " + where + ", and has no flag for another component"
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
	return spelling + " was removed in Astro CLI v2: use --" + name
}

// findRemovedFlag returns the entry for the flag called name that applies on
// cmd, or nil when none does.
func findRemovedFlag(cmd *cobra.Command, name string) *removedFlag {
	// `astro local stop -f`: stop still has --force, without the -f, so
	// "--force was removed" would be false there.
	if cmd.Flag(name) != nil {
		return nil
	}
	for i := range removedFlags {
		f := &removedFlags[i]
		if f.name != name {
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
func forceRemoved(cmd *cobra.Command, typed string) string {
	yes := cmd.Flag("yes")
	spelled := "--yes"
	if yes.Shorthand != "" {
		spelled += " (-" + yes.Shorthand + ")"
	}
	// Changed alone would include --yes=false, which skips nothing.
	if yes.Changed && yes.Value.String() == "true" {
		return "--force was removed in Astro CLI v2: " + spelled + ", which you passed, already skips the confirmation, so drop " + typed
	}
	return "--force was removed in Astro CLI v2: use " + spelled
}

// formatRemoved names the formats cmd's -o takes, which differ by command
// (dotenv on `env variable get` and `list`), and says what became of yaml,
// which 1.x's --format took everywhere.
func formatRemoved(cmd *cobra.Command, _ string) string {
	msg := "--format was removed in Astro CLI v2: use -o (--output), which takes " + cliout.FormatList(cmd)
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
// same arity, which offers nothing and runs nothing, since a completion
// request never runs the command (acceptForCompletion).
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
		if findRemovedFlag(cmd, on[i].name) != nil {
			addHiddenFlag(cmd, &on[i])
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

func shorthandFlag(cmd *cobra.Command, letter string) *pflag.Flag {
	if f := cmd.LocalFlags().ShorthandLookup(letter); f != nil {
		return f
	}
	return cmd.InheritedFlags().ShorthandLookup(letter)
}
