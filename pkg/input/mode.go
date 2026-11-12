package input

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mattn/go-isatty"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/pkg/telemetry"
)

// NonInteractiveFlag is the name of the global flag that turns prompts off.
const NonInteractiveFlag = "non-interactive"

// The reasons a run may not ask, as a refusal words them.
const (
	reasonJSON = "with --output json it cannot"
	reasonFlag = "with --non-interactive it cannot"
	reasonCI   = "with CI set it cannot"

	// A question needs a person at a terminal: stdin to answer on, stderr to
	// see it on.
	reasonStdin  = "stdin is not a terminal"
	reasonStderr = "stderr is not a terminal"

	// CI systems that do not set CI themselves, which telemetry already
	// recognizes by name.
	reasonJenkins = "in Jenkins (JENKINS_URL or HUDSON_URL set) it cannot"
	reasonAzure   = "in Azure Pipelines (TF_BUILD set) it cannot"

	falseValue = "false"
)

// AddFlag registers the global --non-interactive flag on fs, the root's
// persistent flags, so every command at every depth accepts it.
func AddFlag(fs *pflag.FlagSet) {
	fs.Bool(NonInteractiveFlag, false, "Never ask a question: a command that needs an answer fails and names the flag that gives it. Always on with CI set, --output json, or stdin or stderr not a terminal; --non-interactive=false asks even with CI set, at a terminal")
}

// Refusal is the one place a run's interaction mode is decided. It returns why
// this run may not ask a question, or "" when it may; the reason finishes the
// sentence "this command needs to ask X; ..." in the refusal.
//
// A run is non-interactive when --output is json; when stdin or stderr is not a
// terminal, since a question needs a person at both, one to answer and one to
// see it (so an answer is never read from a pipe: a script passes the flag);
// when the CI environment variable is set to a truthy value (anything but
// empty, "0" or "false", in any case); when Jenkins (JENKINS_URL) or Azure
// Pipelines (TF_BUILD, truthy) says so, which do not set CI; or when
// --non-interactive is passed. An explicit --non-interactive=false asks again
// despite CI, so a person can debug a CI setup at a terminal, but never under
// json and never without both terminals.
//
// flag is the parsed --non-interactive flag, or nil where the command has none.
func Refusal(json bool, flag *pflag.Flag) string {
	if json {
		return reasonJSON
	}
	asked := flag != nil && flag.Changed
	if asked && flag.Value.String() != falseValue {
		return reasonFlag
	}
	if !StdinIsTerminal() {
		return reasonStdin
	}
	if !StderrIsTerminal() {
		return reasonStderr
	}
	if asked {
		return ""
	}
	switch {
	case truthy(os.Getenv("CI")):
		return reasonCI
	case telemetry.IsCISystemSet("jenkins"):
		return reasonJenkins
	case telemetry.IsCISystemSet("azure-devops") && truthy(os.Getenv("TF_BUILD")):
		return reasonAzure
	}
	return ""
}

// truthy is a CI value that says CI is on: anything but empty, "0" or "false".
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", falseValue:
		return false
	}
	return true
}

// CanAsk reports whether this run may ask a question at all (see Refusal): the
// one accessor for a command deciding whether to prompt or to fail for a
// missing answer. A run with no guard installed (a test, a library caller) may.
func CanAsk() bool { return refusal() == "" }

// terminals is the answer a test gives for stdin and stderr, in place of the
// real ones: nil means ask the real ones.
var terminals atomic.Pointer[[2]bool]

// StdinIsTerminal reports whether stdin is a terminal: the one answer to the
// question, which Refusal asks and which a command that only needs to know
// (not to decide whether to ask) may too. A Cygwin or MSYS pty (mintty) counts.
func StdinIsTerminal() bool {
	if t := terminals.Load(); t != nil {
		return t[0]
	}
	if testing.Testing() {
		return true
	}
	return isTerminal(os.Stdin)
}

// StderrIsTerminal is StdinIsTerminal for stderr, where a question is drawn.
func StderrIsTerminal() bool {
	if t := terminals.Load(); t != nil {
		return t[1]
	}
	if testing.Testing() {
		return true
	}
	return isTerminal(os.Stderr)
}

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
