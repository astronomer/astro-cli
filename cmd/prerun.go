package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func formatConstraint(c string) string {
	return "<" + c
}

func formatMajor(l string) string {
	return l[:strings.LastIndex(l, ".")]
}

func getConstraint(c string) *semver.Constraints {
	nc, err := semver.NewConstraint(c)
	if err != nil {
		// TODO: Handle constraint not being parsable.
		return nil
	}
	return nc
}

func getVersion(cv string) *semver.Version {
	v, err := semver.NewVersion(cv)
	if err != nil {
		// TODO: Handle cv not being parsable.
		return nil
	}
	return v
}

// PersistentPreRunCheck for validation of the CLI vs Deployment Version
func PersistentPreRunCheck(client *houston.Client, cmd *cobra.Command, out io.Writer) {
	ac := deployment.GetAppConfig()

	// Skip check if AppConfig doesn't exist
	if ac != nil {
		dv := ac.Version
		cv := version.CurrVersion
		fc := formatConstraint(dv)
		p := getConstraint(fc)

		fm := formatMajor(dv)
		fc = formatConstraint(fm)
		m := getConstraint(fc)

		v := getVersion(cv)

		if m.Check(v) {
			color.Red("There is a major update for astro-cli. You're using %s and %s is the latest.  Please upgrade to the latest before continuing..", cv, dv)
			// TODO: don't exit on auth commands logout or login
			os.Exit(1)
		} else if p.Check(v) {
			color.Yellow("A new patch of Astronomer is available. Your version is %s and %s is the latest.", cv, dv)
		}
	}
}
