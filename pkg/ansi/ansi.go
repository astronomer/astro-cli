package ansi

import (
	"os"
	"strconv"

	fatihcolor "github.com/fatih/color"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/pkg/input"
)

// ForceColors forces the use of colors and other ANSI sequences.
var ForceColors = false

// EnvironmentOverrideColors overs coloring based on `CLICOLOR` and
// `CLICOLOR_FORCE`. Cf. https://bixense.com/clicolors/
var EnvironmentOverrideColors = true

var color = Color()

const (
	cliColorForce = "CLICOLOR_FORCE"
	forceColor    = "FORCE_COLOR"
)

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

// noColorFlag is set by --no-color; see SetNoColor.
var noColorFlag bool

// SetNoColor turns color off for the rest of the run, over every environment
// variable: it is what --no-color does, and it takes effect when the flag is
// parsed, before the command (or its error) prints anything.
func SetNoColor() {
	noColorFlag = true
	color = Color()
	fatihcolor.NoColor = true
}

// NoColorFlag is the name of the global flag that turns color off.
const NoColorFlag = "no-color"

type noColorValue struct{}

func (noColorValue) String() string   { return strconv.FormatBool(noColorFlag) }
func (noColorValue) Type() string     { return "bool" }
func (noColorValue) IsBoolFlag() bool { return true }
func (noColorValue) Set(v string) error {
	on, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	if on {
		SetNoColor()
	}
	return nil
}

// AddNoColorFlag registers the global --no-color flag on fs, the root's
// persistent flags. The flag turns color off as soon as it is parsed, so it
// also covers an error raised before the command runs.
func AddNoColorFlag(fs *pflag.FlagSet) {
	f := fs.VarPF(noColorValue{}, NoColorFlag, "", "Disable colored output. NO_COLOR does the same")
	f.NoOptDefVal = "true"
}

// shouldColor decides colors for a stream. In order: --no-color; a non-empty
// NO_COLOR (no-color.org), which beats everything below it; FORCE_COLOR and
// CLICOLOR_FORCE, where "0" means off and anything else non-empty means on;
// CLICOLOR=0; then whether the stream is a terminal, which TERM=dumb is not.
func shouldColor(isTerminal func() bool) bool {
	if noColorFlag {
		return false
	}
	if EnvironmentOverrideColors {
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		for _, name := range []string{forceColor, cliColorForce} {
			switch os.Getenv(name) {
			case "":
			case "0":
				return false
			default:
				return true
			}
		}
		if os.Getenv("CLICOLOR") == "0" {
			return false
		}
		if os.Getenv("TERM") == "dumb" && !ForceColors {
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
