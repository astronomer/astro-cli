package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	apcCmd "github.com/astronomer/astro-cli/cmd/apc"
	"github.com/astronomer/astro-cli/cmd/api"
	astroCmd "github.com/astronomer/astro-cli/cmd/astro"
	"github.com/astronomer/astro-cli/cmd/local"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/internal/telemetry"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

var (
	verboseLevel   string
	houstonClient  houston.ClientInterface
	houstonVersion string
)

const (
	apcPlatform   = "APC"
	cloudPlatform = "Astro"
)

// rootOptions is everything the root command reads from the environment: which
// platform this machine points at, whether it holds a usable session, and where
// output goes. Holding them apart from the assembly is what lets a test build
// either platform branch with no ambient config and no network.
type rootOptions struct {
	platform      string // cloudPlatform or apcPlatform
	loggedIn      bool
	houstonClient houston.ClientInterface
	out           io.Writer
}

// detectRootOptions reads the machine. It is the only ambient part of building
// the root, so newRootCmd below stays a pure function of what this returns.
func detectRootOptions() rootOptions {
	platform := cloudPlatform
	if !context.IsCloudContext() {
		platform = apcPlatform
	}
	return rootOptions{
		platform:      platform,
		loggedIn:      true,
		houstonClient: houston.NewClient(houston.NewHTTPClient()),
		out:           os.Stdout,
	}
}

// NewRootCmd adds all of the primary commands for the cli
func NewRootCmd() *cobra.Command {
	return newRootCmd(detectRootOptions())
}

func newRootCmd(o rootOptions) *cobra.Command {
	// Cobra sorts a parent's children alphabetically the first time it renders
	// them, latching the result. Turning it off makes registration order the
	// rendered order everywhere, which is what lets the root menu below read in
	// the order someone works in rather than the alphabet. It is set here, at
	// the top of assembly, because the sort latches on the first Commands()
	// call: set any later and which subtrees are sorted depends on which
	// construction helper happened to walk them first.
	cobra.EnableCommandSorting = false

	// cmd/auth.go reads both of these package-level values, so the assembly
	// still publishes them rather than keeping them local.
	houstonClient = o.houstonClient
	houstonVersion = ""

	astroV1Client := astrov1.NewV1Client(httputil.NewHTTPClient())
	v1Alpha1Client := astrov1alpha1.NewV1Alpha1Client(httputil.NewHTTPClient())

	isCloudCtx := o.platform == cloudPlatform
	if !isCloudCtx {
		version, err := houstonClient.GetPlatformVersion(nil)
		if err != nil {
			apcCmd.InitDebugLogs = append(apcCmd.InitDebugLogs, fmt.Sprintf("Unable to get Houston version: %s", err.Error()))
		}
		houstonVersion = version
	}

	rootCmd := &cobra.Command{
		Use:   "astro",
		Short: "Run Apache Airflow locally and interact with Astronomer",
		// The ASCII banner above this is drawn by the help func (rootBanner),
		// not kept here, because Long is wrapped and the art is not prose.
		Long: "Welcome to the Astro CLI, the modern command line interface for data orchestration. You can use it for Astro, APC, or Local Development.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip heavy pre-run logic for commands that opt out via annotation
			if cmd.Annotations[telemetry.SkipPreRunAnnotation] == "true" {
				return nil
			}
			return utils.ChainRunEs(
				SetupLogging,
				CreateRootPersistentPreRunE(astroV1Client),
				telemetry.CreateTrackingHook(),
			)(cmd, args)
		},
	}

	rootCmd.AddCommand(
		newLoginCommand(astroV1Client, o.out),
		newLogoutCommand(o.out),
		newAuthRootCmd(astroV1Client, o.out),
		newVersionCommand(),
		newContextCmd(astroV1Client, o.out),
		newConfigRootCmd(o.out),
		api.NewAPICmd(),
		newTelemetryCmd(o.out),
		newTelemetrySendCmd(),
		newOttoCmd(),
	)

	if isCloudCtx { // Include all the commands to be exposed for cloud users
		rootCmd.AddCommand(
			astroCmd.AddCmds(astroV1Client, v1Alpha1Client, v1Alpha1Client, o.out)...,
		)
	} else { // Include all the commands to be exposed for APC users
		rootCmd.AddCommand(
			apcCmd.AddCmds(houstonClient, o.out)...,
		)
		apcCmd.VersionMatchCmds(rootCmd, []string{"astro"})
	}

	// The core tree (`astro local`, `astro init`, the start/stop/logs aliases,
	// and the `astro dev` removal stub) mounts outside the cloud/software
	// branch: local Airflow works offline with no account. Every command
	// carries the skip-pre-run annotation — cmd/local's TestTreeInvariants
	// checks that structurally — so PersistentPreRunE above returns before
	// the logging setup, the platform pre-run and the telemetry hook, which
	// is where the network calls are. The stub replaces the 1.x `astro dev`
	// tree in this binary.
	//
	// The home config is still read, for core commands too: main calls
	// config.InitConfig before this function runs, and it has to, because
	// detectRootOptions asks context.IsCloudContext() which of the two
	// subtrees above to mount — the shape of the tree depends on the config
	// file before cobra has seen argv. The read creates nothing, so a core
	// command on a machine that never logged in leaves no config/ state behind
	// (config.initHome, and TestInitLeavesNoHomeConfigBehind in e2e).
	coreDeps := local.NewDeps()
	wireLinkPickers(&coreDeps, o.platform, astroV1Client, o.out)
	// A single positional argument is Otto's first message in an interactive
	// session.
	coreDeps.LaunchOtto = func(prompt string) error { return launchOtto([]string{prompt}) }
	// The deploy this tree mounted is where `astro init` assumes a converted
	// project deploys when the project does not say: APC's builds its 1.x
	// Dockerfile, so it is kept, and a saved target is not made an Astro link.
	coreDeps.DeploysToAPC = !isCloudCtx
	rootCmd.AddCommand(local.AddCmds(coreDeps)...)

	groupCommands(rootCmd)
	// A flag the CLI does not have is recorded here, before cliout reports it:
	// cobra parses flags before the tracking hook in PersistentPreRunE runs,
	// so a mistyped flag would otherwise send nothing. cliout.Execute keeps
	// this func and runs it ahead of its own (markUsageErrors). An unknown
	// command is recorded by Execute, after the run it failed.
	rootCmd.SetFlagErrorFunc(trackUnknownFlag)
	rootCmd.PersistentFlags().StringVarP(&verboseLevel, "verbosity", "", logrus.WarnLevel.String(), "Log level (debug, info, warn, error, fatal, panic)")
	installHelp(rootCmd, o.platform, houstonVersion)

	return rootCmd
}

const (
	groupDevelop = "develop"
	groupInspect = "inspect"
	groupShip    = "ship"
	groupManage  = "manage"
	groupCLI     = "cli"
)

// commandGroups is the root menu: which groups there are, what order they read
// in, and what order the commands inside each one read in.
//
// It is one table rather than a GroupID set at each command's construction,
// because the root tree is assembled from four packages across two platform
// branches and deciding where a command belongs means seeing the alternatives
// next to each other. A command missing from the table renders under
// "Additional Commands", so a new one nobody classified says so rather than
// landing somewhere plausible and wrong.
//
// The order inside each group is the order someone works in, not the alphabet:
// you package before you deploy, and you log in before you manage anything.
var commandGroups = []struct {
	id    string
	title string
	names []string
}{
	{groupDevelop, "Develop locally:", []string{"init", "local", "start", "stop", "logs"}},
	{groupInspect, "Inspect Airflow:", []string{"af", "use", "link", "api"}},
	{groupShip, "Ship:", []string{"package", "deploy", "remote", "dbt"}},
	{groupManage, "Manage Astro:", []string{
		"login", "logout", "auth", "context",
		"organization", "workspace", "deployment", "env",
		"user", "team", "ide",
	}},
	// help and completion are not listed: cobra injects them at execute time,
	// so they are absent from the constructed tree. SetHelpCommandGroupID and
	// SetCompletionCommandGroupID below put them in this group.
	{groupCLI, "Set up the CLI:", []string{"config", "telemetry", "otto", "version"}},
}

// commandGroup answers which group a command belongs to, built from the table
// above so the two cannot disagree.
var commandGroup = func() map[string]string {
	m := map[string]string{}
	for _, g := range commandGroups {
		for _, name := range g.names {
			m[name] = g.id
		}
	}
	return m
}()

// commandOrder answers where a command sits inside its group.
var commandOrder = func() map[string]int {
	m := map[string]int{}
	for _, g := range commandGroups {
		for i, name := range g.names {
			m[name] = i
		}
	}
	return m
}()

func groupCommands(rootCmd *cobra.Command) {
	for _, g := range commandGroups {
		rootCmd.AddGroup(&cobra.Group{ID: g.id, Title: g.title})
	}
	// Cobra sorts a parent's children alphabetically when it renders them, and
	// there is no per-command hook to change that, so the order has to be
	// imposed on the slice itself. Sorting stays on for every other parent in
	// the tree; only the root menu is ordered by hand.
	subs := rootCmd.Commands()
	sort.SliceStable(subs, func(i, j int) bool {
		return commandOrder[subs[i].Name()] < commandOrder[subs[j].Name()]
	})
	rootCmd.RemoveCommand(subs...)
	for _, cmd := range subs {
		cmd.GroupID = commandGroup[cmd.Name()]
	}
	rootCmd.AddCommand(subs...)
	rootCmd.SetHelpCommandGroupID(groupCLI)
	rootCmd.SetCompletionCommandGroupID(groupCLI)
}

// commandSpellings is every word a command answers to, as a command list
// prints it: "organization, org".
//
// Cobra shows a command's aliases only on that command's own help page, so
// `astro org` was reachable but discoverable nowhere. Naming them in the list
// is what docker and cargo do, and it is the whole reason to have them.
func commandSpellings(cmd *cobra.Command) string {
	spellings := cmd.Name()
	for _, alias := range cmd.Aliases {
		spellings += ", " + alias
	}
	return spellings
}
