package ansi

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/pkg/input"
)

// GuidanceDisabledEnv names the variable that turns off optional next steps:
// "1" or "true" (any case), as every on/off ASTRO_* switch accepts. Required
// next steps, warnings and errors still print, and so does every hint field
// in --output json.
const GuidanceDisabledEnv = "ASTRO_GUIDANCE_DISABLED"

// MaxNextSteps is the most steps PrintNextSteps lists before it counts the
// rest.
const MaxNextSteps = 5

// NextStep is one command a run suggests, and what running it gets the
// reader ("Set API_URL so astro local start can run"). Why is a sentence
// fragment with no trailing punctuation, and may be empty for a command that
// needs no reason.
type NextStep struct {
	Why     string
	Command string
}

// NextSteps is what a run suggests doing after it, printed by
// PrintNextSteps.
type NextSteps struct {
	// Steps are the commands, the one that recovers from a blocking state
	// first.
	Steps []NextStep
	// More says where the steps past MaxNextSteps are, after "and N more":
	// Why alone ("marked in the STATUS column above"), or with the Command
	// that lists them all.
	More NextStep
	// Required is for a step the run cannot be finished without, such as
	// deploying Dags a setting change left undeployed. It prints wherever
	// the run's stderr goes, and ASTRO_GUIDANCE_DISABLED does not hide it.
	// Every other step is optional guidance.
	Required bool
}

// GuidanceDisabled reports whether ASTRO_GUIDANCE_DISABLED turns optional
// next steps off.
func GuidanceDisabled() bool {
	v := os.Getenv(GuidanceDisabledEnv)
	return v == "1" || strings.EqualFold(v, "true")
}

// PrintNextSteps writes n to w, which is stderr: next steps are not a
// command's result, so stdout stays exactly that. A caller under
// --output json prints none, and carries its commands in the JSON instead.
//
// Optional steps print only when the process's stderr is a terminal and
// ASTRO_GUIDANCE_DISABLED is not set; that is the question asked whatever w
// is, since w stands for stderr, so a pipe, a script or an agent that
// opted out never sees them. Required steps always print.
//
// The output is set off by a blank line and led by the Next: label. One step
// with no reason stays on the label's line:
//
//	Next: astro local start
//
// Otherwise each step is its reason, then its command on a line of its own,
// so a command is never wrapped into prose:
//
//	Next:
//	  Set API_URL so astro local start can run:
//	    astro local env variable set API_URL
//	  and 3 more, marked in the STATUS column above.
func PrintNextSteps(w io.Writer, n NextSteps) {
	if len(n.Steps) == 0 {
		return
	}
	if !n.Required && (GuidanceDisabled() || !input.StderrIsTerminal()) {
		return
	}
	label := NextLabel(w)
	if len(n.Steps) == 1 && n.Steps[0].Why == "" {
		fmt.Fprintf(w, "\n%s %s\n", label, n.Steps[0].Command)
		return
	}
	var b strings.Builder
	b.WriteString("\n" + label + "\n")
	shown := n.Steps
	if len(shown) > MaxNextSteps {
		shown = shown[:MaxNextSteps]
	}
	for _, s := range shown {
		writeNextStep(&b, "  ", s)
	}
	if rest := len(n.Steps) - len(shown); rest > 0 {
		more := n.More
		more.Why = fmt.Sprintf("and %d more", rest) + prefixed(", ", more.Why)
		writeNextStep(&b, "  ", more)
	}
	fmt.Fprintf(w, "%s", b.String())
}

// writeNextStep writes one step at indent: its reason and, under it, its
// command, or whichever of the two it has. A reason alone is a sentence and
// ends with a period.
func writeNextStep(b *strings.Builder, indent string, s NextStep) {
	switch {
	case s.Why == "":
		b.WriteString(indent + s.Command + "\n")
	case s.Command == "":
		b.WriteString(indent + s.Why + ".\n")
	default:
		b.WriteString(indent + s.Why + ":\n" + indent + "  " + s.Command + "\n")
	}
}

// prefixed is s led by sep, or nothing when s is empty.
func prefixed(sep, s string) string {
	if s == "" {
		return ""
	}
	return sep + s
}
