package ansi

import (
	"os"

	"github.com/logrusorgru/aurora"

	"github.com/astronomer/astro-cli/pkg/input"
)

// ForceColors forces the use of colors and other ANSI sequences.
var ForceColors = false

// EnvironmentOverrideColors overs coloring based on `CLICOLOR` and
// `CLICOLOR_FORCE`. Cf. https://bixense.com/clicolors/
var EnvironmentOverrideColors = true

var color = Color()

const cliColorForce = "CLICOLOR_FORCE"

// Bold returns bolded text if the writer supports colors.
//
// Every styling function here takes text as text, not a format (a name with
// a % in it prints as written), and styles it as input.Lines has it: what is
// styled is most often a name from an API, and an escape sequence in it would
// be acted on by the terminal, inside a question about that very name. Line
// breaks are kept, since a prompt may start on a fresh line.
func Bold(text string) string {
	return color.Bold(input.Lines(text)).String()
}

// Color returns an aurora.Aurora instance with colors enabled or disabled
// depending on whether the writer supports colors.
func Color() aurora.Aurora {
	return aurora.NewAurora(shouldUseColors())
}

// Green returns text colored green
func Green(text string) string {
	return color.Green(input.Lines(text)).String()
}

// Cyan returns text colored blue
func Cyan(text string) string {
	return color.Cyan(input.Lines(text)).String()
}

func shouldUseColors() bool {
	return shouldColor(IsOutputTerminal)
}

// shouldColor decides colors for a stream, by the environment's override and
// otherwise by whether the stream is a terminal.
func shouldColor(isTerminal func() bool) bool {
	if EnvironmentOverrideColors {
		force, ok := os.LookupEnv(cliColorForce)

		if ok && force != "0" {
			return true
		}
		if ok && force == "0" {
			return false
		}
		if os.Getenv("CLICOLOR") == "0" {
			return false
		}
		// NO_COLOR, set and not empty, turns color off for every stream
		// (no-color.org); only an explicit CLICOLOR_FORCE, above, wins over it.
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
	}

	return ForceColors || isTerminal()
}

// Palette colors text for one stream.
type Palette struct{ a aurora.Aurora }

// ForStderr colors text written to stderr, where every question and its
// notes go: by whether stderr is a terminal, not stdout. A prompt seen at the
// terminal stays colored with stdout redirected, and one written to a
// redirected stderr carries no escapes. It is decided at each call, since a
// prompt is built just before it is asked.
func ForStderr() Palette {
	return Palette{aurora.NewAurora(shouldColor(isMessagesTerminal))}
}

// Bold returns text bolded for the palette's stream.
func (p Palette) Bold(text string) string { return p.a.Bold(input.Lines(text)).String() }

// Red returns text colored red for the palette's stream.
func (p Palette) Red(text string) string { return p.a.Red(input.Lines(text)).String() }

// Green returns text colored green for the palette's stream.
func (p Palette) Green(text string) string { return p.a.Green(input.Lines(text)).String() }

// Cyan returns text colored cyan for the palette's stream.
func (p Palette) Cyan(text string) string { return p.a.Cyan(input.Lines(text)).String() }

// Yellow returns text colored yellow for the palette's stream.
func (p Palette) Yellow(text string) string { return p.a.Yellow(input.Lines(text)).String() }
