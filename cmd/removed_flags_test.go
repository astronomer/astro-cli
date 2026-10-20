package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrocontext "github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The trees the removed-flag tests run: Astro for a non-hosted organization
// and for a hosted one, and APC at the newest platform version, where every
// version-gated command is mounted. Each test builds its own, so it runs
// against that tree's state.
var (
	astroTree       = treeConfig{name: "astro", platform: cloudPlatform}
	astroHostedTree = treeConfig{name: "astro hosted", platform: cloudPlatform, hosted: true}
	apcTree         = treeConfig{name: "apc " + newestAPCVersion, platform: apcPlatform, apcVersion: newestAPCVersion}
)

// v1Trees pairs each tree of v1_flags.tsv with the v2 tree a script written
// against it now runs.
var v1Trees = []struct {
	v1   string
	tree treeConfig
}{
	{v1TreeAstro, astroTree},
	{v1TreeAstroHosted, astroHostedTree},
	{v1TreeAPC, apcTree},
}

var errRan = errors.New("the command got past flag parsing")

// disarmedTree builds c's tree with every hook and run replaced, so a run
// that gets past flag parsing ends with errRan before it logs in, prompts or
// asks an API anything. preRuns counts the pre-runs that started.
func disarmedTree(t *testing.T, c treeConfig) (root *cobra.Command, preRuns *int) {
	t.Helper()
	root, _, preRuns = disarmedTreeKeeping(t, c, func(*cobra.Command) bool { return false })
	return root, preRuns
}

// disarmedTreeKeeping is disarmedTree leaving a command keep reports to run as
// it does in production: its own hooks and run, those of everything below it,
// and the pre-runs above it, which run for it as they would. Any other
// command they run is still disarmed. out is the writer the root was built
// with, where a command publishes its result.
func disarmedTreeKeeping(t *testing.T, c treeConfig, keep func(*cobra.Command) bool) (root *cobra.Command, out *bytes.Buffer, preRuns *int) {
	t.Helper()
	tree := buildTree(t, c)
	root, out = tree.root, tree.out
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	preRuns = new(int)
	// The pre-runs as built, which a kept command gets from the nearest
	// command that has one, as cobra gives it.
	original := map[*cobra.Command]func(*cobra.Command, []string) error{}
	var disarm func(*cobra.Command)
	disarm = func(cmd *cobra.Command) {
		if keep(cmd) {
			return
		}
		original[cmd] = cmd.PersistentPreRunE
		self := cmd
		cmd.PersistentPreRun, cmd.PreRun, cmd.PreRunE = nil, nil, nil
		cmd.PersistentPreRunE = func(run *cobra.Command, args []string) error {
			if keep(run) {
				for p := self; p != nil; p = p.Parent() {
					if pre := original[p]; pre != nil {
						return pre(run, args)
					}
				}
				return nil
			}
			*preRuns++
			return errRan
		}
		if cmd.Runnable() {
			cmd.Run = nil
			cmd.RunE = func(*cobra.Command, []string) error { return errRan }
		}
		for _, sub := range cmd.Commands() {
			disarm(sub)
		}
	}
	disarm(root)
	return root, out, preRuns
}

// removedFlagCase is a run passing a removed flag on a command that had it
// in 1.x and does not now, and what it must be told.
type removedFlagCase struct {
	tree treeConfig
	args []string // the command, and its arguments before the flag
	flag string   // the removed flag's name
	// value is a value to pass the flag, for one that took one in 1.x;
	// empty for a 1.x boolean.
	value     string
	shorthand string
	// want is what the run is told after "<flag> was removed in Astro CLI
	// v2: ", the flag spelled as the run typed it.
	want string
}

// removedFlagCases holds at least one case for every entry in removedFlags
// (TestRemovedFlagsSayWhatReplacedThem checks that), with the reason it
// gives there.
var removedFlagCases = []removedFlagCase{
	{tree: astroTree, args: []string{"deployment", "create", "--name", "x"}, flag: "deployment-file", value: "deployment.yaml", want: errDeploymentFileRemoved},
	{tree: astroHostedTree, args: []string{"deployment", "update", "dep-id"}, flag: "deployment-file", value: "deployment.yaml", want: errDeploymentFileRemoved},
	{tree: astroTree, args: []string{"deployment", "inspect", "dep-id"}, flag: "template", shorthand: "t", want: errInspectTemplateRemoved},
	{tree: astroTree, args: []string{"organization", "switch", "my-org"}, flag: "login-link", shorthand: "l", want: errLoginLinkRemoved},
	{tree: astroTree, args: []string{"api", "airflow", "GET", "/dags"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: astroTree, args: []string{"api", "airflow", "ls"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: apcTree, args: []string{"api", "airflow", "describe", "get_dags"}, flag: "api-url", value: "http://localhost:8080", want: errAPIURLFlagRemoved},
	{tree: astroTree, args: []string{"api", "airflow"}, flag: "deployment-id", value: "dep-id", want: errDeploymentIDAPIFlag},
	{tree: apcTree, args: []string{"api", "airflow", "ls"}, flag: "deployment-id", value: "dep-id", want: errDeploymentIDAPIFlag},
	{tree: astroTree, args: []string{"env", "list"}, flag: "workspace-id", value: "ws-id", want: errEnvListWorkspaceIDRemoved},
	{tree: astroHostedTree, args: []string{"env", "list"}, flag: "deployment-id", value: "dep-id", want: errEnvListDeploymentIDRemoved},
	{tree: astroTree, args: []string{"workspace", "list"}, flag: "json", want: errJSONFlagRemoved},
	{tree: astroHostedTree, args: []string{"deployment", "list", "-a"}, flag: "json", want: errJSONFlagRemoved},
	{tree: astroTree, args: []string{"api", "cloud", "ls"}, flag: "json", want: errJSONFlagRemoved},
	{tree: astroTree, args: []string{"deployment", "list"}, flag: "template", value: "{{.}}", want: errTemplateFlagRemoved},
	{tree: astroTree, args: []string{"organization", "team", "list"}, flag: "template", value: "{{.}}", want: errTemplateFlagRemoved},
	{
		tree: astroTree, args: []string{"env", "variable", "list"}, flag: "format", value: "yaml",
		want: "use -o (--output), which takes text, json or dotenv; there is no yaml, so use -o json",
	},
	{
		tree: astroTree, args: []string{"env", "connection", "get", "my_conn"}, flag: "format", value: "json",
		want: "use -o (--output), which takes text or json; there is no yaml, so use -o json",
	},
	{tree: astroTree, args: []string{"deployment", "delete", "dep-id"}, flag: "force", shorthand: "f", want: "use --yes (-y)"},
	{tree: astroHostedTree, args: []string{"deployment", "bundle", "delete"}, flag: "force", shorthand: "f", want: "use --yes (-y)"},
	{tree: apcTree, args: []string{"deployment", "update", "dep-id"}, flag: "force", shorthand: "f", want: "use --yes (-y)"},
	{tree: astroTree, args: []string{"deploy", "dep-id"}, flag: "save", shorthand: "s", want: errDeploySaveRemoved},
	{tree: apcTree, args: []string{"deploy", "dep-id"}, flag: "save", shorthand: "s", want: errDeploySaveRemoved},
	{tree: astroTree, args: []string{"deploy", "dep-id"}, flag: "pytest", want: errDeployPytestRemoved},
	{tree: astroHostedTree, args: []string{"deploy", "dep-id"}, flag: "test", value: "tests/test_dags.py", shorthand: "t", want: errDeployTestRemoved},
	{tree: astroTree, args: []string{"deploy", "dep-id"}, flag: "env", value: ".env.ci", shorthand: "e", want: errDeployEnvRemoved},
	{tree: astroTree, args: []string{"deploy", "dep-id"}, flag: "parse", want: errDeployParseRemoved},
	{tree: astroTree, args: []string{"deploy", "dep-id", "--dags"}, flag: "dags-path", value: "./elsewhere", want: errDeployDagsPathRemoved},
	{tree: astroTree, args: []string{"deploy", "dep-id"}, flag: "dag-bundle-name", value: "nightly", want: errDeployBundleNameRemoved},
	{tree: astroTree, args: []string{"deploy"}, flag: "deployment-name", value: "prod", shorthand: "n", want: errDeployNameRemoved},
	{tree: apcTree, args: []string{"deploy", "dep-id"}, flag: "no-cache", want: errAPCDeployNoCache},
	{tree: astroTree, args: []string{"deploy", "dep-id"}, flag: "non-dags", want: errDeployNonDagsRemoved},
	{tree: astroHostedTree, args: []string{"deploy"}, flag: "non-dags-mount-path", value: "/usr/local/airflow/x", want: errDeployNonDagsMountPathRemoved},
	{tree: astroTree, args: []string{"deploy"}, flag: "non-dags-local-path", value: "./analytics", want: errDeployNonDagsLocalPathRemoved},
	{tree: astroTree, args: []string{"deploy"}, flag: "non-dags-bundle-type", value: "dbt", want: errDeployNonDagsBundleTypeRemoved},
	{tree: astroTree, args: []string{"deploy", "dep-id"}, flag: "build-secrets", value: "id=mysecret,src=secrets.txt", want: errBuildSecretsRemoved},
	{tree: astroHostedTree, args: []string{"remote", "deploy"}, flag: "build-secrets", value: "id=mysecret,src=secrets.txt", want: errBuildSecretsRemoved},
	// The flags 1.x kept hidden and deprecated.
	{tree: astroTree, args: []string{"deployment", "update", "dep-id"}, flag: "enforce-cicd", want: errEnforceCICDRemoved},
	{tree: astroHostedTree, args: []string{"deployment", "create", "--name", "x"}, flag: "enforce-cicd", want: errEnforceCICDRemoved},
	{tree: astroHostedTree, args: []string{"deployment", "create", "--name", "x"}, flag: "cluster-type", value: "dedicated", want: errClusterTypeRemoved},
	{tree: astroTree, args: []string{"deploy", "dep-id"}, flag: "force-upgrade-to-af3", want: errForceUpgradeToAF3Remove},
	{tree: apcTree, args: []string{"deployment", "delete", "dep-id"}, flag: "hard", want: errHardDeleteRemoved},

	// astro dev init's, on astro init.
	{tree: astroTree, args: []string{"init"}, flag: "force", shorthand: "f", want: errInitForceRemoved},
	{tree: apcTree, args: []string{"init", "my-project"}, flag: "force", shorthand: "f", want: errInitForceRemoved},
	{tree: astroTree, args: []string{"init"}, flag: "from-template", shorthand: "t", want: errInitTemplateRemoved},
	{tree: astroTree, args: []string{"init"}, flag: "runtime-version", value: "12.0.0", shorthand: "v", want: errInitRuntimeRemoved},
	{tree: apcTree, args: []string{"init"}, flag: "runtime-version", value: "12.0.0", shorthand: "v", want: errInitRuntimeRemoved},
	{tree: astroTree, args: []string{"init"}, flag: "remote-execution-enabled", want: errInitRemoteExecRemoved},
	{tree: astroHostedTree, args: []string{"init"}, flag: "remote-image-repository", value: "quay.io/acme/client", want: errInitRemoteRepoRemoved},
	{tree: apcTree, args: []string{"init"}, flag: "use-astronomer-certified", want: errInitACRemoved},
	{tree: astroTree, args: []string{"init"}, flag: "docker", want: errInitDockerRemoved},
	{tree: astroTree, args: []string{"init"}, flag: "standalone", want: errInitStandaloneRemoved},

	// astro dev start's and restart's, on astro local start and restart, and
	// the root's astro start.
	{tree: astroTree, args: []string{"local", "start"}, flag: "standalone", want: errStartStandaloneRemoved},
	{tree: astroTree, args: []string{"start"}, flag: "standalone", want: errStartStandaloneRemoved},
	{
		tree: astroTree, args: []string{"local", "restart"}, flag: "docker",
		want: "astro local restart keeps the mode Airflow is running in. To change it, use astro local stop, then astro local start --docker",
	},
	{
		tree: astroTree, args: []string{"local", "restart"}, flag: "standalone",
		want: "astro local restart keeps the mode Airflow is running in. To change it, use astro local stop, then astro local start",
	},
	{tree: astroTree, args: []string{"local", "restart"}, flag: "port", value: "8081", shorthand: "p", want: errRestartPortRemoved},
	{tree: astroTree, args: []string{"local", "restart"}, flag: "kill", shorthand: "k", want: errRestartKillRemoved},
	{tree: astroTree, args: []string{"local", "start"}, flag: "compose-file", value: "compose.yaml", want: errStartComposeFileRemoved},
	{tree: astroTree, args: []string{"local", "start"}, flag: "deployment-id", value: "dep-id", shorthand: "d", want: errStartDeploymentIDRemoved},
	{tree: astroTree, args: []string{"local", "restart"}, flag: "workspace-id", value: "ws-id", shorthand: "w", want: errStartWorkspaceIDRemoved},
	{
		tree: astroTree, args: []string{"local", "start"}, flag: "foreground", shorthand: "f",
		want: "astro local start returns once Airflow is up, and leaves it running. Follow its logs with astro local logs -f, and stop it with astro local stop",
	},
	{
		tree: astroTree, args: []string{"local", "restart"}, flag: "foreground", shorthand: "f",
		want: "astro local restart returns once Airflow is up, and leaves it running. Follow its logs with astro local logs -f, and stop it with astro local stop",
	},
	{tree: astroTree, args: []string{"local", "start"}, flag: "image-name", value: "my-image:1", shorthand: "i", want: errStartImageNameRemoved},
	{tree: astroTree, args: []string{"local", "start"}, flag: "no-browser", shorthand: "n", want: errStartNoBrowserRemoved},
	{tree: apcTree, args: []string{"start"}, flag: "no-browser", shorthand: "n", want: errStartNoBrowserRemoved},
	{tree: astroTree, args: []string{"local", "start"}, flag: "no-proxy", want: errStartNoProxyRemoved},
	{tree: astroTree, args: []string{"local", "start"}, flag: "settings-file", value: "settings.yaml", shorthand: "s", want: errStartSettingsRemoved},
	{
		tree: astroTree, args: []string{"local", "start"}, flag: "wait", value: "5m",
		want: "set the ASTRO_LOCAL_HEALTH_TIMEOUT environment variable to a Go duration instead, as in ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro local start",
	},
	{
		tree: astroTree, args: []string{"start"}, flag: "wait", value: "5m",
		want: "set the ASTRO_LOCAL_HEALTH_TIMEOUT environment variable to a Go duration instead, as in ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro start",
	},
	{
		tree: astroTree, args: []string{"local", "start"}, flag: "env", value: ".env.local", shorthand: "e",
		want: "astro local start reads the .env at the project root. To set a value there, use astro local env variable set <NAME> --plain",
	},
	{
		tree: astroTree, args: []string{"local", "check"}, flag: "env", value: ".env.local", shorthand: "e",
		want: "astro local check reads the .env at the project root. To set a value there, use astro local env variable set <NAME> --plain",
	},
	{
		tree: astroTree, args: []string{"local", "start"}, flag: "no-cache",
		want: "standalone mode builds no image, and Docker mode (--docker) builds on the container engine's cache, which a changed dependency or file invalidates. " +
			"To build from nothing, clear the engine's build cache first, with docker builder prune",
	},
	{
		tree: apcTree, args: []string{"start"}, flag: "no-cache",
		want: "standalone mode builds no image, and Docker mode (--docker) builds on the container engine's cache, which a changed dependency or file invalidates. " +
			"To build from nothing, clear the engine's build cache first, with docker builder prune",
	},
	{
		tree: astroTree, args: []string{"local", "restart"}, flag: "no-cache",
		want: "standalone mode builds no image, and Docker mode builds on the container engine's cache, which a changed dependency or file invalidates. " +
			"To build from nothing, clear the engine's build cache first, with docker builder prune",
	},
	{
		tree: astroTree, args: []string{"package"}, flag: "no-cache",
		want: "astro package builds on the container engine's cache, which a changed dependency or file invalidates. " +
			"To build from nothing, clear the engine's build cache first, with docker builder prune",
	},

	// astro dev parse's, on astro local check.
	{
		tree: astroTree, args: []string{"local", "check"}, flag: "docker",
		want: "astro local check parses the Dags in the project's own environment and builds no image, so drop it",
	},
	{
		tree: astroTree, args: []string{"local", "check"}, flag: "standalone",
		want: "astro local check parses the Dags in the project's own environment and builds no image, so drop it",
	},
	{
		tree: astroTree, args: []string{"local", "check"}, flag: "image-name", value: "my-image:1", shorthand: "i",
		want: "astro local check parses the Dags in the project's own environment and builds no image, so drop it",
	},
	{
		tree: astroTree, args: []string{"local", "check"}, flag: "build-secret", value: "id=mysecret,src=secrets.txt",
		want: "astro local check parses the Dags in the project's own environment and builds no image, so drop it",
	},
	{
		tree: astroTree, args: []string{"local", "check"}, flag: "build-secrets", value: "id=mysecret,src=secrets.txt",
		want: "astro local check parses the Dags in the project's own environment and builds no image, so drop it",
	},

	// astro dev build's, on astro package.
	{
		tree: astroTree, args: []string{"package"}, flag: "docker",
		want: "astro package builds the same image whichever mode Airflow runs in locally, so drop it",
	},
	{
		tree: astroTree, args: []string{"package"}, flag: "standalone",
		want: "astro package builds the same image whichever mode Airflow runs in locally, so drop it",
	},
	{tree: astroTree, args: []string{"package"}, flag: "image-name", value: "my-image:1", shorthand: "i", want: errPackageImageNameRemoved},

	// The commands that act on whatever is running, or was.
	{tree: astroTree, args: []string{"local", "stop"}, flag: "docker", want: "astro local stop works in either mode, so drop it"},
	{tree: astroTree, args: []string{"stop"}, flag: "standalone", want: "astro stop works in either mode, so drop it"},
	{tree: astroTree, args: []string{"local", "status"}, flag: "docker", want: "astro local status works in either mode, so drop it"},
	{tree: astroTree, args: []string{"local", "reset"}, flag: "standalone", want: "astro local reset works in either mode, so drop it"},

	// astro dev logs' components, and astro dev bash's containers.
	{tree: astroTree, args: []string{"local", "logs"}, flag: "api-server", shorthand: "a", want: "use --component api-server"},
	{tree: astroTree, args: []string{"local", "logs"}, flag: "dag-processor", shorthand: "d", want: "use --component dag-processor"},
	{tree: astroTree, args: []string{"logs"}, flag: "scheduler", shorthand: "s", want: "use --component scheduler"},
	{tree: astroTree, args: []string{"local", "logs", "-f"}, flag: "triggerer", shorthand: "t", want: "use --component triggerer"},
	{tree: apcTree, args: []string{"local", "logs"}, flag: "webserver", shorthand: "w", want: "use --component webserver"},
	{tree: astroTree, args: []string{"local", "shell"}, flag: "api-server", shorthand: "a", want: shellElsewhere},
	{tree: astroTree, args: []string{"local", "shell"}, flag: "dag-processor", shorthand: "d", want: shellElsewhere},
	{tree: astroTree, args: []string{"local", "shell"}, flag: "postgres", shorthand: "p", want: shellElsewhere},
	{
		tree: astroTree, args: []string{"local", "shell"}, flag: "scheduler", shorthand: "s",
		want: "astro local shell opens its shell in the scheduler's container in Docker mode, and in the project's environment in standalone mode, so drop it",
	},
	{tree: astroTree, args: []string{"local", "shell"}, flag: "triggerer", shorthand: "t", want: shellElsewhere},
	{tree: astroTree, args: []string{"local", "shell"}, flag: "webserver", shorthand: "w", want: shellElsewhere},
}

// shellElsewhere is what astro local shell says of a 1.x flag that picked a
// container other than the scheduler's.
const shellElsewhere = "astro local shell opens its shell in the scheduler's container in Docker mode, " +
	"and in the project's environment in standalone mode, and has no flag for another component"

// typedFlag is one way to type a case's flag, and the flag as the message
// names it.
type typedFlag struct {
	args  []string
	typed string
}

// spellings is every way the case's flag can be typed: --name, --name=value
// and --name value (or the boolean alone), the shorthand, and the shorthand
// first in a group, before a letter that would ask for help.
func (c *removedFlagCase) spellings() []typedFlag {
	value := c.value
	if value == "" {
		value = "true"
	}
	long := "--" + c.flag
	out := []typedFlag{{[]string{long + "=" + value}, long}}
	if c.value != "" {
		out = append(out, typedFlag{[]string{long, c.value}, long})
	} else {
		out = append(out, typedFlag{[]string{long}, long})
	}
	if short := "-" + c.shorthand; c.shorthand != "" {
		out = append(out, typedFlag{[]string{short}, short}, typedFlag{[]string{short + "h"}, short})
	}
	return out
}

// Every entry, every spelling, in text and under --output json: a usage error
// with the entry's message, led by the flag as typed, before any pre-run, and
// under json the one error object on stdout.
func TestRemovedFlagsSayWhatReplacedThem(t *testing.T) {
	reached := map[*removedFlag]bool{}
	for i := range removedFlagCases {
		c := &removedFlagCases[i]
		for _, spelling := range c.spellings() {
			want := spelling.typed + " was removed in Astro CLI v2: " + c.want
			for _, asJSON := range []bool{false, true} {
				args := append(append([]string{}, c.args...), spelling.args...)
				if asJSON {
					args = append(args, "-o", "json")
				}
				name := c.tree.name + ": astro " + strings.Join(args, " ")
				t.Run(name, func(t *testing.T) {
					root, preRuns := disarmedTree(t, c.tree)
					target, _, err := root.Find(c.args)
					require.NoError(t, err)
					if f := findRemovedFlag(target, c.flag); f != nil {
						reached[f] = true
					}
					if asJSON && !cliout.HasOutput(target) {
						t.Skip("no --output here: an astro api request prints the API's response")
					}

					stdout, stderr, err := executeRoot(root, args...)
					require.Error(t, err)
					assert.Equal(t, want, err.Error())
					assert.Equal(t, cliout.ExitUsage, cliout.ExitCode(t.Context(), err))
					assert.Zero(t, *preRuns, "a pre-run started before the refusal")
					if !asJSON {
						assert.Empty(t, stdout)
						assert.Contains(t, stderr, "Error: "+want)
						return
					}
					assert.Empty(t, stderr)
					require.Equal(t, 1, strings.Count(stdout, "\n"), "stdout is not one line: %q", stdout)
					var obj map[string]any
					require.NoError(t, json.Unmarshal([]byte(stdout), &obj), stdout)
					assert.Equal(t, map[string]any{"error": want, "code": float64(cliout.ExitUsage), "kind": string(cliout.KindUsage)}, obj)
				})
			}
		}
	}
	for i := range removedFlags {
		assert.True(t, reached[&removedFlags[i]], "removedFlags[%d] (--%s) has no case in removedFlagCases", i, removedFlags[i].name)
	}
}

// --force's message changes when the run passed --yes already, ahead of it:
// pflag has set -y by the time it reaches the f of -yf. After it (-fy), the
// run has not got there yet, and is told to use what it is about to pass.
func TestRemovedForceSaysWhenYesIsAlreadyThere(t *testing.T) {
	const use = " was removed in Astro CLI v2: use --yes (-y)"
	const passed = " was removed in Astro CLI v2: --yes (-y), which you passed, already skips the confirmation, so drop it"
	for _, tc := range []struct {
		flags []string
		want  string
	}{
		{[]string{"-yf"}, "-f" + passed},
		{[]string{"-y", "-f"}, "-f" + passed},
		{[]string{"--yes", "--force"}, "--force" + passed},
		{[]string{"--yes=true", "--force"}, "--force" + passed},
		{[]string{"-fy"}, "-f" + use},
		{[]string{"--force", "--yes"}, "--force" + use},
		{[]string{"--yes=false", "--force"}, "--force" + use},
		{[]string{"--yes=false", "-f"}, "-f" + use},
		{[]string{"-fh"}, "-f" + use},
	} {
		args := append([]string{"deployment", "delete", "dep-id"}, tc.flags...)
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root, preRuns := disarmedTree(t, astroTree)
			_, _, err := executeRoot(root, args...)
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			assert.True(t, cliout.IsUsage(err))
			assert.Zero(t, *preRuns)
		})
	}
}

// Each entry's under names a command a tree has, so an entry cannot quietly
// stop applying when a command is renamed.
func TestRemovedFlagsNameRealCommands(t *testing.T) {
	trees := rootsUnderTest(t)
	for _, f := range removedFlags {
		for _, path := range f.under {
			found := false
			for _, tree := range trees {
				cmd, _, err := tree.root.Find(strings.Fields(path))
				if err == nil && isUnder(cmd, path) && strings.HasSuffix(cmd.CommandPath(), path) {
					found = true
				}
			}
			assert.True(t, found, "--%s: no tree has astro %s", f.name, path)
		}
	}
}

// A command that still has the flag runs with it, and one that did not have
// it in 1.x is told what cobra always told it: "was removed" would be false
// there, or name the wrong flag (-f meant other things on other commands).
func TestRemovedFlagsLeaveOtherCommandsAlone(t *testing.T) {
	for _, tc := range []struct {
		tree treeConfig
		args []string
		want string // the error; errRan when flag parsing succeeded
	}{
		// These still have the flag.
		{astroTree, []string{"deploy", "--force"}, errRan.Error()},
		{astroTree, []string{"deploy", "-f"}, errRan.Error()},
		{astroTree, []string{"local", "stop", "--force"}, errRan.Error()},
		{astroTree, []string{"login", "--login-link"}, errRan.Error()},
		{astroTree, []string{"api", "airflow", "--template", "{{.}}"}, errRan.Error()},
		{apcTree, []string{"deployment", "team", "list", "--deployment-id", "dep-id"}, errRan.Error()},
		// 1.x had no `local` commands. astro dev stop and kill, which local stop
		// and reset replaced (v1Predecessors), had none of these flags.
		{astroTree, []string{"local", "stop", "-f"}, "unknown shorthand flag: 'f' in -f"},
		{astroTree, []string{"local", "reset", "-f"}, "unknown shorthand flag: 'f' in -f"},
		{astroTree, []string{"local", "reset", "--force"}, "unknown flag: --force"},
		{astroTree, []string{"local", "reset", "--yes", "--force"}, "unknown flag: --force"},
		{astroTree, []string{"stop", "--clean", "-f"}, "unknown shorthand flag: 'f' in -f"},
		// astro dev start and init's flags that their replacements still
		// have, without the letter 1.x gave them.
		{astroTree, []string{"local", "start", "-p", "8081"}, "-p was removed in Astro CLI v2: use --port"},
		{astroTree, []string{"start", "-p", "8081"}, "-p was removed in Astro CLI v2: use --port"},
		{astroTree, []string{"init", "-n", "my-project"}, "-n was removed in Astro CLI v2: use --name"},
		{apcTree, []string{"init", "-a", "3.1"}, "-a was removed in Astro CLI v2: use --airflow-version"},
		// And still have, full stop.
		{astroTree, []string{"local", "start", "--port", "8081", "--docker"}, errRan.Error()},
		{astroTree, []string{"local", "logs", "-f"}, errRan.Error()},
		{astroTree, []string{"init", "--name", "my-project", "--airflow-version", "3.1"}, errRan.Error()},
		// 1.x had no `local run`'s flags to report: astro dev run parsed
		// none, so they are the command's arguments.
		{astroTree, []string{"local", "run", "--docker"}, "unknown flag: --docker"},
		// Commands 1.x had, without these flags.
		{astroTree, []string{"version", "--force"}, "unknown flag: --force"},
		{astroTree, []string{"deploy", "--json"}, "unknown flag: --json"},
		{astroTree, []string{"deployment", "logs", "dep-id", "--deployment-id", "x"}, "unknown flag: --deployment-id"},
		{apcTree, []string{"deployment", "create", "--deployment-file", "f.yaml"}, "unknown flag: --deployment-file"},
		{apcTree, []string{"deployment", "delete", "dep-id", "--force"}, "unknown flag: --force"},
		{astroTree, []string{"deployment", "list", "-t"}, "unknown shorthand flag: 't' in -t"},
	} {
		t.Run(tc.tree.name+": astro "+strings.Join(tc.args, " "), func(t *testing.T) {
			root, _ := disarmedTree(t, tc.tree)
			_, _, err := executeRoot(root, tc.args...)
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
		})
	}
}

// A removed flag is still an unknown flag to telemetry: the events are what
// tell us when nobody passes one any more, and its entry can go.
func TestRemovedFlagsStillReachTelemetry(t *testing.T) {
	type event struct{ command, flag string }
	var got []event
	orig := recordUnknownFlag
	recordUnknownFlag = func(cmd *cobra.Command, flag string) { got = append(got, event{cmd.CommandPath(), flag}) }
	t.Cleanup(func() { recordUnknownFlag = orig })

	for _, args := range [][]string{
		{"deployment", "delete", "dep-id", "--force"},
		{"deployment", "delete", "dep-id", "-f"},
		{"workspace", "list", "--json", "-o", "json"},
	} {
		root, _ := disarmedTree(t, astroTree)
		_, _, err := executeRoot(root, args...)
		require.ErrorContains(t, err, "was removed in Astro CLI v2", args)
	}
	assert.Equal(t, []event{
		{"astro deployment delete", "--force"},
		{"astro deployment delete", "-f"},
		{"astro workspace list", "--json"},
	}, got)
}

// Asking for help gets the help, wherever the removed flag stands. Only a
// shorthand group that reaches the removed letter before the h is refused,
// as pflag stops there.
func TestRemovedFlagsGiveWayToHelp(t *testing.T) {
	for _, args := range [][]string{
		{"deployment", "list", "--json", "--help"},
		{"deployment", "list", "--help", "--json"},
		{"deployment", "list", "--json", "-h"},
		{"deployment", "list", "--json", "-h", "-o", "json"},
		{"deployment", "list", "--json", "--help=true"},
		{"deployment", "list", "--json", "-h=true"},
		{"deployment", "delete", "dep-id", "-f", "-h"},
		{"deployment", "delete", "dep-id", "-hf"},
		{"init", "--runtime-version", "12.0.0", "--help"},
		{"local", "start", "-n", "-h"},
		{"init", "-n", "my-project", "-h"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root, preRuns := disarmedTree(t, astroTree)
			var out strings.Builder
			root.SetOut(&out)
			stdout, _, err := executeRootKeepingOut(root, args...)
			require.NoError(t, err)
			assert.Empty(t, stdout)
			target, _, err := root.Find(args)
			require.NoError(t, err)
			assert.Contains(t, out.String(), "Usage:\n  "+target.UseLine(), "no help for %s", target.CommandPath())
			assert.Zero(t, *preRuns)
		})
	}
	// A -h that is a value is still read as asking for help, and the worst
	// that does is show help: the flag stays refused for a run that is going
	// to run.
	root, _ := disarmedTree(t, astroTree)
	_, _, err := executeRoot(root, "deployment", "list", "--json", "--", "-h")
	require.EqualError(t, err, removedMessage("--json", errJSONFlagRemoved))
	// --help=false asks for nothing, before the removed flag or after it.
	for _, args := range [][]string{
		{"deployment", "list", "--json", "--help=false"},
		{"deployment", "list", "--help=false", "--json"},
	} {
		root, _ := disarmedTree(t, astroTree)
		_, _, err := executeRoot(root, args...)
		require.EqualError(t, err, removedMessage("--json", errJSONFlagRemoved), strings.Join(args, " "))
	}
}

func TestHasHelpFlag(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"x", "--help=true"}, {"-h=1"}, {"--help=TRUE"}} {
		assert.True(t, hasHelpFlag(args), args)
	}
	for _, args := range [][]string{nil, {"--help=false"}, {"-h=0"}, {"--help=maybe"}, {"--", "-h"}, {"-hf"}, {"--helpful"}} {
		assert.False(t, hasHelpFlag(args), args)
	}
}

// executeRootKeepingOut runs root like executeRoot, but leaves the out writer
// the test set on it, where cobra prints help.
func executeRootKeepingOut(root *cobra.Command, args ...string) (stdout, stderr string, err error) {
	var out, errOut strings.Builder
	root.SetErr(&errOut)
	err = execute(context.Background(), root, args, &out)
	return out.String(), errOut.String(), err
}

// A shell asking for completions after a removed flag gets them: cobra
// parses the flags of the command it completes without the flag error func,
// and would fail the completion on any flag it does not know.
//
// The command completed is the one cobra finds, as for a run. A removed flag
// before a subcommand's name that cobra takes for a flag with a value (the
// subcommand's name) leaves the parent to complete, which never had the
// flag, so it offers nothing, as a run of that line is refused. Neither
// panics or fails the request.
func TestRemovedFlagsLeaveCompletionsAlone(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		want    string
		offered bool
	}{
		{[]string{"__complete", "deployment", "list", "--json", "-"}, "--all", true},
		{[]string{"__complete", "deployment", "list", "--json", "--o"}, "--output", true},
		{[]string{"__completeNoDesc", "deployment", "list", "--template", "{{.}}", "--o"}, "--output", true},
		{[]string{"__complete", "deployment", "delete", "-f", "--y"}, "--yes", true},
		{[]string{"__complete", "deployment", "token", "delete", "-f", "--y"}, "--yes", true},
		{[]string{"__complete", "api", "airflow", "ls", "--json", "--fi"}, "--filter", true},
		{[]string{"__complete", "api", "airflow", "describe", "--api-url", "http://localhost:8080", "--re"}, "--refresh", true},
		// cobra takes the URL for --api-url's value here, and finds ls.
		{[]string{"__complete", "api", "airflow", "--api-url", "http://localhost:8080", "ls", "--fi"}, "--filter", true},
		{[]string{"__complete", "api", "airflow", "--api-url=http://localhost:8080", "ls", "--fi"}, "--filter", true},
		// A shorthand 1.x gave a flag the command still has under its long
		// name alone, which a run is told to spell out.
		{[]string{"__complete", "init", "-n", "my-project", "--a"}, "--airflow-version", true},
		{[]string{"__complete", "init", "-a", "3.1", "--n"}, "--name", true},
		{[]string{"__complete", "local", "start", "-p", "8080", "--d"}, "--docker", true},
		{[]string{"__complete", "start", "-p", "8080", "--d"}, "--docker", true},

		{[]string{"__complete", "api", "airflow", "--json", "ls", "--fi"}, "--filter", false},
		{[]string{"__complete", "deployment", "-f", "delete", "--y"}, "--yes", false},
		{[]string{"__complete", "deployment", "token", "-f", "delete", "--y"}, "--yes", false},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			root, _ := disarmedTree(t, astroTree)
			// cobra's __complete runs under the root's pre-run, which the real
			// one lets through and the disarmed one would not.
			root.PersistentPreRunE = nil
			// cobra writes completions to the completed command's out, which
			// a command built with a writer of its own does not inherit.
			var out strings.Builder
			var setOut func(*cobra.Command)
			setOut = func(cmd *cobra.Command) {
				cmd.SetOut(&out)
				for _, sub := range cmd.Commands() {
					setOut(sub)
				}
			}
			setOut(root)
			var stderr string
			var err error
			require.NotPanics(t, func() { _, stderr, err = executeRootKeepingOut(root, tc.args...) })
			require.NoError(t, err)
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			require.NotEmpty(t, lines)
			offered := make([]string, 0, len(lines)-1)
			for _, l := range lines[:len(lines)-1] {
				offered = append(offered, strings.SplitN(l, "\t", 2)[0])
			}
			if !tc.offered {
				assert.NotContains(t, offered, tc.want)
				return
			}
			assert.NotContains(t, stderr, "Error")
			assert.NotEqual(t, ":1", lines[len(lines)-1], "cobra reported an error: %s", stderr)
			assert.Contains(t, offered, tc.want)
		})
	}
}

// Every command 1.x had takes its removed flags for a completion, and the
// shorthands 1.x gave flags it still has under their long names alone,
// without a name or shorthand clashing with one it has (pflag panics on
// that), and then parses each of them.
func TestEveryV1CommandCompletesPastItsRemovedFlags(t *testing.T) {
	for _, tc := range v1Trees {
		root := buildTree(t, tc.tree).root
		accepts := func(cmd *cobra.Command, path, arg string) {
			require.NotPanics(t, func() { acceptForCompletion(root, append(strings.Fields(path), arg)) }, "%s: %s", tc.tree.name, path)
			assert.NoError(t, cmd.ParseFlags([]string{arg}), "%s: astro %s %s", tc.tree.name, path, arg)
		}
		check := func(cmd *cobra.Command, path string, f *v1Flag) {
			value := ""
			if !f.isBool {
				value = "=x"
			}
			if cmd.Flag(f.name) == nil {
				accepts(cmd, path, "--"+f.name+value)
				return
			}
			require.NotPanics(t, func() { acceptForCompletion(root, append(strings.Fields(path), "--"+f.name+value)) }, "%s: %s", tc.tree.name, path)
			if f.shorthand != "" && f.shorthand != "h" && shorthandFlag(cmd, f.shorthand) == nil {
				accepts(cmd, path, "-"+f.shorthand+value)
			}
		}
		for _, f := range v1Flags() {
			if !slices.Contains(f.trees, tc.v1) {
				continue
			}
			cmd, _, err := root.Find(strings.Fields(f.path))
			if err != nil || pathBelowRoot(cmd) != f.path || cmd.DisableFlagParsing {
				continue
			}
			check(cmd, f.path, &f)
		}
		// And every command that replaced one under another name, past the
		// removed flags its predecessor had.
		for path, was := range v1Predecessors {
			cmd, _, err := root.Find(strings.Fields(path))
			require.NoError(t, err, path)
			for _, f := range v1FlagsAt(v1Flags(), tc.v1, []string{was}) {
				check(cmd, path, &f)
			}
		}
	}
}

// v1Predecessors names, for each v2 command, the astro dev command that
// scaffold.DevReplacements says it replaced, or for the root's start, stop
// and logs, the one their astro local command did; and every astro dev
// command the mapping sends to an astro command is in it, but for the ones
// its comment leaves out.
func TestV1PredecessorsFollowTheDevMapping(t *testing.T) {
	leftOut := map[string]bool{"run": true, "object export": true, "object": true}
	replacedBy := map[string]string{}
	for _, m := range scaffold.DevReplacements() {
		words, ok := strings.CutPrefix(m.Replacement, "astro ")
		if !ok {
			continue // uv run pytest
		}
		path := strings.Join(slices.DeleteFunc(strings.Fields(words), func(w string) bool { return strings.HasPrefix(w, "-") }), " ")
		replacedBy["dev "+m.Command] = path
		if !leftOut[m.Command] {
			assert.Equal(t, "dev "+m.Command, v1Predecessors[path], "astro %s replaced astro dev %s", path, m.Command)
		}
	}
	for path, was := range v1Predecessors {
		local, ok := replacedBy[was]
		if assert.True(t, ok, "astro %s: no row of the dev mapping is astro %s", path, was) && local != path {
			assert.Equal(t, "local "+path, local, "astro %s: astro %s replaced astro %s", path, local, was)
		}
	}
}

// A completion request that passes no flag its command lacks never reads the
// inventory, and one that does reads it.
func TestCompletionsReadTheInventoryOnlyForAnUnknownFlag(t *testing.T) {
	read := false
	orig := v1Flags
	v1Flags = func() []v1Flag { read = true; return orig() }
	t.Cleanup(func() { v1Flags = orig })

	for _, tc := range []struct {
		args []string
		read bool
	}{
		{[]string{"deployment", "list", "--all", "-o", "json"}, false},
		{[]string{"api", "airflow", "--deployment", "dep-id", "ls", "--refresh"}, false},
		{[]string{"deployment", "delete", "dep-id", "-y"}, false},
		{[]string{"deployment", "list", "--json"}, true},
		{[]string{"api", "airflow", "--json", "ls"}, true},
		{[]string{"deployment", "delete", "dep-id", "-yf"}, true},
	} {
		read = false
		root := buildTree(t, astroTree).root
		acceptForCompletion(root, tc.args)
		assert.Equal(t, tc.read, read, strings.Join(tc.args, " "))
	}
}

// Every root says which 1.x tree a script on its machine was written for,
// and whether that is Astro's for a hosted organization is read from the
// context when it is asked, not when the root is built.
func TestEveryTreeNamesItsV1Tree(t *testing.T) {
	for _, tc := range v1Trees {
		root := buildTree(t, tc.tree).root
		assert.Equal(t, tc.v1, v1TreeOf(root), tc.tree.name)
	}
	root := buildTree(t, astroTree).root
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.CloudPlatform) })
	ctx, err := astrocontext.GetCurrentContext()
	require.NoError(t, err)
	require.NoError(t, ctx.SetContextKey("organization_product", "HOSTED"))
	assert.Equal(t, v1TreeAstroHosted, v1TreeOf(root), "a root built before the context said hosted")
}

// A 1.x path a v2 command answers to by an alias finds the flags 1.x had
// there, as does the v2 name.
func TestV1FlagsMatchAliasedPaths(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	group := &cobra.Command{Use: "deployment", Aliases: []string{"deployments"}}
	renamed := &cobra.Command{Use: "variable", Aliases: []string{"airflow-variable"}}
	root.AddCommand(group)
	group.AddCommand(renamed)
	assert.Equal(t, []string{
		"deployment variable", "deployment airflow-variable",
		"deployments variable", "deployments airflow-variable",
	}, pathSpellings(renamed))

	flags := []v1Flag{
		{trees: []string{v1TreeAstro}, path: "deployment airflow-variable", name: "json", isBool: true},
		{trees: []string{v1TreeAPC}, path: "deployment airflow-variable", name: "force", isBool: true},
		{trees: []string{v1TreeAstro}, path: "deployment pool", name: "template"},
	}
	on := v1FlagsAt(flags, v1TreeAstro, pathSpellings(renamed))
	require.Len(t, on, 1)
	assert.Equal(t, "json", on[0].name)
	assert.Empty(t, v1FlagsAt(flags, v1TreeAstro, pathSpellings(group)))
}

// v1_flags.tsv reads the same with CRLF line ends, as a Windows checkout
// without .gitattributes' eol=lf would embed it, and a file that does not
// parse is an error here rather than a panic in a run.
func TestV1FlagsFileParsesWithCRLF(t *testing.T) {
	lf, err := parseV1Flags(v1FlagsFile)
	require.NoError(t, err)
	crlf, err := parseV1Flags(strings.ReplaceAll(v1FlagsFile, "\n", "\r\n"))
	require.NoError(t, err)
	assert.Equal(t, lf, crlf)

	const malformed = "astro\tdeployment list\tjson\t\tmaybe\n"
	_, err = parseV1Flags(malformed)
	require.ErrorContains(t, err, "malformed line")
	assert.Nil(t, loadV1Flags(malformed))
	assert.NotEmpty(t, loadV1Flags(strings.ReplaceAll(v1FlagsFile, "\n", "\r\n")))

	// With no inventory, a removed flag is an unknown flag, as cobra says,
	// and a completion request is left as cobra would take it.
	orig := v1Flags
	v1Flags = func() []v1Flag { return loadV1Flags(malformed) }
	t.Cleanup(func() { v1Flags = orig })
	root, _ := disarmedTree(t, astroTree)
	_, _, err = executeRoot(root, "deployment", "delete", "dep-id", "--force")
	require.EqualError(t, err, "unknown flag: --force")
	assert.True(t, cliout.IsUsage(err))
	root = buildTree(t, astroTree).root
	assert.NotPanics(t, func() { acceptForCompletion(root, []string{"api", "airflow", "--json", "ls", "--force"}) })
}

// v1_flags.tsv parses, names only the trees there are, and is in order, so a
// regenerated one diffs line by line. It is checked in with LF line ends,
// which .gitattributes keeps on every checkout.
func TestV1FlagsFileIsWellFormed(t *testing.T) {
	assert.NotContains(t, v1FlagsFile, "\r", "v1_flags.tsv has CRLF line ends: is it still text eol=lf in .gitattributes?")
	flags, err := parseV1Flags(v1FlagsFile)
	require.NoError(t, err)
	require.Greater(t, len(flags), 900)
	known := []string{v1TreeAstro, v1TreeAstroHosted, v1TreeAPC}
	seen := map[string]bool{}
	var lines []string
	for _, f := range flags {
		for _, tree := range f.trees {
			assert.Contains(t, known, tree, "%s --%s", f.path, f.name)
			seen[tree] = true
		}
		lines = append(lines, f.path+"\t"+f.name+"\t"+f.shorthand)
	}
	assert.Len(t, seen, len(known), "a tree has no flags")
	assert.True(t, slices.IsSorted(lines), "v1_flags.tsv is not sorted by command path and flag")
}

// Every flag 1.x had, on a command v2 still has, either still parses there or
// fails with what replaced it, word for word, so a script written for 1.x is
// never told only "unknown flag". So does every flag 1.x had on a command v2
// replaced under another name (v1Predecessors), on the replacement. A flag v2
// drops without an entry in removedFlags fails here, as does an entry no 1.x
// flag reaches. A command v2 dropped altogether is its own tombstone (`astro
// dev`, `astro run`, `env ... create`), and is skipped.
func TestEveryV1FlagStillWorksOrSaysWhatReplacedIt(t *testing.T) {
	reached := map[*removedFlag]bool{}
	for _, tc := range v1Trees {
		root, preRuns := disarmedTree(t, tc.tree)
		c := &v1FlagChecker{t: t, root: root, preRuns: preRuns, reached: reached}
		for _, f := range v1Flags() {
			if !slices.Contains(f.trees, tc.v1) {
				continue
			}
			label := tc.tree.name + ": astro " + f.path + " --" + f.name
			cmd, rest, err := root.Find(strings.Fields(f.path))
			if !assert.NoError(t, err, label) {
				continue
			}
			if len(rest) > 0 {
				// The words left over name no command under the deepest one
				// that matched: a dropped command, which is its own
				// tombstone (`astro dev`), or a rename that kept no alias.
				assert.True(t, cmd.Hidden, "%s: 1.x's path resolves to `%s` with %v left over; a renamed command should keep its 1.x name as an alias", label, cmd.CommandPath(), rest)
				continue
			}
			// A command renamed with its 1.x name kept as an alias is reached
			// at the 1.x path, and checked there.
			if !assert.Contains(t, pathSpellings(cmd), f.path, label) || cmd.DisableFlagParsing {
				continue
			}
			c.check(cmd, strings.Fields(f.path), &f, label)
		}
		for _, path := range slices.Sorted(maps.Keys(v1Predecessors)) {
			was := v1Predecessors[path]
			cmd, rest, err := root.Find(strings.Fields(path))
			if !assert.NoError(t, err, path) || !assert.Empty(t, rest, "%s: astro %s is not a command", tc.tree.name, path) {
				continue
			}
			require.False(t, cmd.DisableFlagParsing, path)
			for _, f := range v1FlagsAt(v1Flags(), tc.v1, []string{was}) {
				c.check(cmd, strings.Fields(path), &f, tc.tree.name+": astro "+path+" (was astro "+was+") --"+f.name)
			}
		}
		t.Logf("%s: %d flags checked", tc.tree.name, c.checked)
		assert.Greater(t, c.checked, 200, "%s: the inventory matched few commands; is v1_flags.tsv still read right?", tc.tree.name)
	}
	for i := range removedFlags {
		// An entry for a command 1.x did not have is reached by no row of
		// the inventory; TestRemovedFlagsSayWhatReplacedThem runs it.
		if removedFlags[i].v2Only {
			continue
		}
		assert.True(t, reached[&removedFlags[i]], "removedFlags[%d] (--%s) applies to no flag 1.x had on a command v2 has", i, removedFlags[i].name)
	}
}

// v1FlagChecker checks 1.x flags against one v2 tree, recording the entries
// they reach.
type v1FlagChecker struct {
	t       *testing.T
	root    *cobra.Command
	preRuns *int
	reached map[*removedFlag]bool
	checked int
}

// check runs 1.x's flag f, by name and by its shorthand, on cmd, which args
// reach: it parses, or fails with what replaced it.
func (c *v1FlagChecker) check(cmd *cobra.Command, args []string, f *v1Flag, label string) {
	c.checked++
	if cmd.Flag(f.name) == nil {
		spelling := "--" + f.name
		arg := spelling
		if !f.isBool {
			arg += "=x"
		}
		c.reached[assertSaysWhatReplacedIt(c.t, c.root, c.preRuns, cmd, args, f, arg, spelling, label)] = true
	}
	// The shorthand may have moved with a rename (-d is --deployment now, as
	// -d was --deployment-id); if nothing has it, it says what replaced the
	// flag too, or, for a flag the command still has, the long name to use.
	if f.shorthand == "" || shorthandFlag(cmd, f.shorthand) != nil {
		return
	}
	label += " (-" + f.shorthand + ")"
	if cmd.Flag(f.name) != nil {
		assertShorthandNamesTheFlag(c.t, c.root, c.preRuns, args, f, label)
		return
	}
	c.reached[assertSaysWhatReplacedIt(c.t, c.root, c.preRuns, cmd, args, f, "-"+f.shorthand, "-"+f.shorthand, label)] = true
}

// assertShorthandNamesTheFlag runs args with the shorthand 1.x gave f, which
// the command reached by args still has under its long name alone, and checks
// it is refused naming that long name.
func assertShorthandNamesTheFlag(t *testing.T, root *cobra.Command, preRuns *int, args []string, f *v1Flag, label string) {
	t.Helper()
	arg := "-" + f.shorthand
	root.SetArgs(append(slices.Clone(args), arg))
	before := *preRuns
	_, err := root.ExecuteC()
	if !assert.Error(t, err, label) {
		return
	}
	assert.Equal(t, arg+" was removed in Astro CLI v2: use --"+f.name, err.Error(), label)
	assert.True(t, cliout.IsUsage(err), "%s: not a usage error", label)
	assert.Equal(t, before, *preRuns, "%s: a pre-run started", label)
}

// assertSaysWhatReplacedIt runs args, which reach cmd, with arg and checks it is refused with
// the message of the entry for f there, and returns that entry.
func assertSaysWhatReplacedIt(t *testing.T, root *cobra.Command, preRuns *int, cmd *cobra.Command, args []string, f *v1Flag, arg, spelling, label string) *removedFlag {
	t.Helper()
	entry := findRemovedFlag(cmd, f.name)
	if !assert.NotNil(t, entry, "%s: no entry in removedFlags applies", label) {
		return nil
	}
	root.SetArgs(append(slices.Clone(args), arg))
	before := *preRuns
	_, err := root.ExecuteC()
	if !assert.Error(t, err, label) {
		return entry
	}
	assert.Equal(t, entry.message(cmd, spelling), err.Error(), label)
	assert.True(t, strings.HasPrefix(err.Error(), spelling+" was removed in Astro CLI v2: "), "%s: %v", label, err)
	assert.True(t, cliout.IsUsage(err), "%s: not a usage error", label)
	assert.Equal(t, before, *preRuns, "%s: a pre-run started", label)
	return entry
}
