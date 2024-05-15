package ansi

import (
	"os"

	"github.com/logrusorgru/aurora"
)

// ForceColors forces the use of colors and other ANSI sequences.
var ForceColors = false

// EnvironmentOverrideColors overs coloring based on `CLICOLOR` and
// `CLICOLOR_FORCE`. Cf. https://bixense.com/clicolors/
var EnvironmentOverrideColors = true

var color = Color()

const cliColorForce = "CLICOLOR_FORCE"

// Bold returns bolded text if the writer supports colors
func Bold(text string) string {
	return color.Sprintf(color.Bold(text))
}

// Color returns an aurora.Aurora instance with colors enabled or disabled
// depending on whether the writer supports colors.
func Color() aurora.Aurora {
	return aurora.NewAurora(shouldUseColors())
}

// Red returns text colored red
func Red(text string) string {
	return color.Sprintf(color.Red(text))
}

// Green returns text colored green
func Green(text string) string {
	return color.Sprintf(color.Green(text))
}

// Blue returns text colored blue
func Blue(text string) string {
	return color.Sprintf(color.Blue(text))
}

// Cyan returns text colored blue
func Cyan(text string) string {
	return color.Sprintf(color.Cyan(text))
}

func shouldUseColors() bool {
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
	}

	return ForceColors || IsOutputTerminal()
}
