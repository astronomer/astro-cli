package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func formatLtConstraint(c string) string {
	return "< " + c
}

func formatDowngradeConstraint(c string) string {
	return "> " + formatMajor(c)
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

func isBehindMajor(cv string, dv string) bool {
	fm := formatMajor(dv)
	fc := formatLtConstraint(fm)
	m := getConstraint(fc)
	v := getVersion(cv)

	return m.Check(v)
}

func isBehindPatch(cv string, dv string) bool {
	fc := formatLtConstraint(dv)
	p := getConstraint(fc)
	v := getVersion(cv)

	return p.Check(v)
}

func isAheadMajor(cv string, dv string) bool {
	fc := formatDowngradeConstraint(dv)
	s := getConstraint(fc)
	v := getVersion(cv)

	return s.Check(v)
}

// PersistentPreRunCheck for validation of the CLI vs Deployment Version
func PersistentPreRunCheck(client *houston.Client, cmd *cobra.Command, out io.Writer) {
	ac := deployment.GetAppConfig()

	// Skip check if AppConfig ia nil
	if ac != nil {
		dv := ac.Version
		cv := version.CurrVersion

		if isBehindMajor(cv, dv) {
			color.Red(messages.ERROR_NEW_MAJOR_VERSION, cv, dv)
			// Exit for commands that require matching major versions
			os.Exit(1)
		} else if isBehindPatch(cv, dv) {
			color.Yellow(messages.WARNING_NEW_PATCH_VERSION, cv, dv)
		} else if isAheadMajor(cv, dv) {
			color.Yellow(messages.WARNING_DOWNGRADE_VERSION, cv, dv)
		}
	}
}
