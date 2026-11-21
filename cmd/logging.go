package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/logger"
)

// verbosityEnv sets the log level where --verbosity is not given. It reaches
// what a flag cannot: the proxy daemon `astro local start` spawns inherits it,
// and a CI job can set it once for every command it runs.
const verbosityEnv = "ASTRO_VERBOSITY"

// logOutput is where SetupLogging sends the log: stderr as it is when the
// command runs (a test may have swapped it), or a test's buffer.
var logOutput = func() io.Writer { return os.Stderr }

// levelFlag is --verbosity's value. pflag parses it, so a level that is not one
// fails the command line as any other bad flag value does, before anything
// runs.
type levelFlag struct{ level slog.Level }

func (f *levelFlag) String() string {
	return strings.ToLower(f.level.String())
}

func (f *levelFlag) Set(s string) error {
	l, err := logger.ParseLevel(s)
	if err != nil {
		return err
	}
	f.level = l
	return nil
}

func (f *levelFlag) Type() string { return "level" }

func addVerbosityFlag(fs *pflag.FlagSet) {
	fs.Var(&levelFlag{level: logger.DefaultLevel}, verbosityFlag,
		"Log level: debug, info, warn or error. Without the flag, "+verbosityEnv+" or the verbosity setting sets it")
}

// SetupLogging is the root's pre-run step that sets the log level and sends
// the log to stderr: stdout is the command's output, which under
// --output json is one object a script parses. Every command runs it, those
// that skip the rest of the pre-run included.
//
// The level is the first of --verbosity, ASTRO_VERBOSITY and the verbosity
// setting that is set, or warn. A flag is only consulted when it was given
// (pflag's Changed), so `--verbosity warn` wins over a setting of debug.
// A bad ASTRO_VERBOSITY stops the command as a bad --verbosity does, as a usage
// error naming the variable: it was set on purpose, and a typo would otherwise
// leave the run quieter or louder than asked. A bad verbosity setting, written
// to a file some time ago, does not: it is reported, and the next source
// decides.
func SetupLogging(cmd *cobra.Command, _ []string) error {
	logger.SetOutput(logOutput())
	if f := cmd.Root().PersistentFlags().Lookup(verbosityFlag); f != nil && f.Changed {
		logger.SetLevel(f.Value.(*levelFlag).level)
		return nil
	}
	level := logger.DefaultLevel
	var bad []string
	if raw := os.Getenv(verbosityEnv); raw != "" {
		if _, err := logger.ParseLevel(raw); err != nil {
			return cliout.Usage(fmt.Errorf("%s: %w", verbosityEnv, err))
		}
	}
	for _, src := range []struct{ name, value string }{
		{verbosityEnv, os.Getenv(verbosityEnv)},
		{"the verbosity setting", config.CFG.Verbosity.GetString()},
	} {
		if src.value == "" {
			continue
		}
		l, err := logger.ParseLevel(src.value)
		if err != nil {
			bad = append(bad, src.name+": "+err.Error())
			continue
		}
		level = l
		break
	}
	logger.SetLevel(level)
	// Written past the level: a level of error, or a 1.x "fatal", would
	// otherwise hide the very typo that set it.
	for _, b := range bad {
		ansi.Warn(logOutput(), "ignoring "+b)
	}
	return nil
}

// readVerbosity sets --verbosity from a removed command's raw arguments. A
// stub parses no flags (cliout.RemovedCommand), so cobra never sets the
// root's --verbosity for it; this reads it in the spellings pflag accepts for
// it, --verbosity=LEVEL and --verbosity LEVEL, stopping at the "--" that ends
// flags. The last one wins, as it would in pflag. A level that is not one is
// passed over: the stub's whole job is to say what replaced the command, and
// a bad log level should not stop it from saying so.
func readVerbosity(cmd *cobra.Command, args []string) error {
	flags := cmd.Root().PersistentFlags()
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if level, ok := strings.CutPrefix(a, "--"+verbosityFlag+"="); ok {
			_ = flags.Set(verbosityFlag, level) //nolint:errcheck // see the doc comment
		} else if a == "--"+verbosityFlag && i+1 < len(args) {
			i++
			_ = flags.Set(verbosityFlag, args[i]) //nolint:errcheck // see the doc comment
		}
	}
	return nil
}
