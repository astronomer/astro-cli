package input

import (
	"os"
	"strings"

	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/astronomer/astro-cli/pkg/telemetry"
)

// NonInteractiveFlag is the name of the global flag that turns prompts off.
const NonInteractiveFlag = "non-interactive"

// The reasons a run may not ask, as a refusal words them.
const (
	reasonJSON = "with --output json it cannot"
	reasonFlag = "with --non-interactive it cannot"
	reasonCI   = "with CI set it cannot"

	// CI systems that do not set CI themselves, which telemetry already
	// recognizes by name.
	reasonJenkins = "in Jenkins (JENKINS_URL or HUDSON_URL set) it cannot"
	reasonAzure   = "in Azure Pipelines (TF_BUILD set) it cannot"

	falseValue = "false"
)

// AddFlag registers the global --non-interactive flag on fs, the root's
// persistent flags, so every command at every depth accepts it.
func AddFlag(fs *pflag.FlagSet) {
	fs.Bool(NonInteractiveFlag, false, "Never ask a question: a command that needs an answer fails and names the flag that gives it. Always on with CI set or --output json; --non-interactive=false asks even with CI set")
}

// Refusal is the one place a run's interaction mode is decided. It returns why
// this run may not ask a question, or "" when it may; the reason finishes the
// sentence "this command needs to ask X; ..." in the refusal.
//
// A run is non-interactive when --output is json, when the CI environment
// variable is set to a truthy value (anything but empty, "0" or "false", in
// any case), when Jenkins (JENKINS_URL) or Azure Pipelines (TF_BUILD, truthy)
// says so, which do not set CI, or when --non-interactive is passed. An explicit
// --non-interactive=false asks again despite CI, so a person can debug a CI
// setup locally, but never under json. Every other run may ask, whether or not
// stdin is a terminal: the question goes to stderr and the answer is read from
// stdin, where a person may be (ssh without -t, docker exec -i, mintty).
//
// flag is the parsed --non-interactive flag, or nil where the command has none.
func Refusal(json bool, flag *pflag.Flag) string {
	if json {
		return reasonJSON
	}
	if flag != nil && flag.Changed {
		if flag.Value.String() != falseValue {
			return reasonFlag
		}
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

// StdinIsTerminal reports whether stdin is a terminal. It does not decide
// whether to ask (CanAsk does); it is for wording ("input ended" at a terminal,
// "stdin is not interactive" without one) and for a stdin that carries a
// payload rather than an answer.
func StdinIsTerminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
