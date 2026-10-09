package apc

import (
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/logger"
)

var (
	// init debug logs should be used only for logs produced during the CLI-initialization, before the SetUpLogs Method has been called
	InitDebugLogs = []string{}

	houstonClient  houston.ClientInterface
	appConfig      *houston.AppConfig
	houstonVersion string

	workspaceID string
	teamID      string
)

// AddCmds adds all the command initialized in this package for the cmd package to import
//
// It asks the platform for nothing. The commands take their shape (flags,
// examples, feature-flagged subcommands) from appConfig and houstonVersion as
// they stand, which is unset until LoadPlatform runs. The root runs that only
// when NeedsPlatform says the command line is for one of these commands, so a
// command that never talks to Houston does not wait on it (#2289).
func AddCmds(client houston.ClientInterface, out io.Writer) []*cobra.Command {
	houstonClient = client
	return newCmds(out)
}

func newCmds(out io.Writer) []*cobra.Command {
	return []*cobra.Command{
		newDeploymentRootCmd(out),
		newWorkspaceCmd(out),
		NewDeployCmd(out),
		newUserCmd(out),
		newTeamCmd(out),
	}
}

// LoadPlatform asks the platform the current context points at for its version
// and feature flags, which AddCmds builds the commands from.
//
// The version is asked for first, because which app config query fits depends
// on it. When that fails the app config is not asked for at all: it comes from
// the same host, and one that did not answer the first request will not answer
// the second, so asking would only double the wait on a dial timeout.
func LoadPlatform(client houston.ClientInterface) {
	var err error
	houstonVersion, err = client.GetPlatformVersion(nil)
	if err != nil {
		InitDebugLogs = append(InitDebugLogs, fmt.Sprintf("Unable to get Houston version: %s", err.Error()))
		appConfig = nil // nothing this platform said, rather than what an earlier load left
		return
	}
	// There is no clusterID in the GetAppConfig call at this point of lifecycle, so we are getting the app config for the default cluster
	appConfig, err = houston.Call(client.GetAppConfig)(houston.GetAppConfigRequest{})
	if err != nil {
		InitDebugLogs = append(InitDebugLogs, fmt.Sprintf("Error checking feature flag: %s", err.Error()))
	}
}

// PlatformVersion is the version LoadPlatform got, or "" when it has not run
// or the platform did not answer.
func PlatformVersion() string {
	return houstonVersion
}

// NeedsPlatform reports whether words, the command line's words that name a
// command, run one of the commands AddCmds builds, and so whether LoadPlatform
// has to run before they are built. rootFlags are the root's persistent flags,
// which cobra needs to tell a flag's value from a command name (`--verbosity
// debug deployment list`).
//
// It answers by finding the command in a probe tree built from these commands
// alone, the way cobra will find it in the real one. The probe has a stand-in
// for cobra's help command, so `help <command>` asks about the command named,
// wherever on the line it stands.
func NeedsPlatform(words []string, rootFlags *pflag.FlagSet) bool {
	probe := &cobra.Command{Use: "astro"}
	probe.PersistentFlags().AddFlagSet(rootFlags)
	probe.AddCommand(newCmds(io.Discard)...)
	help := &cobra.Command{Use: "help"}
	probe.AddCommand(help)
	found, rest, err := probe.Find(words)
	if err == nil && found == help {
		found, _, err = probe.Find(rest)
	}
	return err == nil && found != probe && found != help
}

// SetUpLogs set the log output and the log level
func SetUpLogs(out io.Writer, level string) error {
	// if level is default means nothing was passed override with config setting
	if level == "warning" {
		level = config.CFG.Verbosity.GetString()
	}
	logger.SetOutput(out)
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	logger.SetLevel(lvl)
	return nil
}

func PrintDebugLogs() {
	for _, log := range InitDebugLogs {
		logger.Debug(log)
	}
	// Free-up memory used by init logs
	InitDebugLogs = nil
}
