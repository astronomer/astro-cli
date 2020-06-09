package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/version"
	"github.com/spf13/cobra"
)

// CheckDeploymentVersion for validation of the CLI vs Deployment Version
func CheckDeploymentVersion(client *houston.Client, cmd *cobra.Command, out io.Writer) {
	appCfg := deployment.GetAppConfig()

	// Skip check if AppConfig is nil
	if appCfg != nil {
		dv := appCfg.Version
		cv := version.CurrVersion

		validateVersions(cv, dv, out)
	}
}

func validateVersions(cv string, dv string, out io.Writer) {
	if isBehindMajor(cv, dv) {
		fmt.Fprintln(out, messages.ERROR_NEW_MAJOR_VERSION, cv, dv)
		// Exit for commands that require matching major versions
		os.Exit(1)
	} else if isBehindPatch(cv, dv) {
		fmt.Fprintln(out, messages.WARNING_NEW_PATCH_VERSION, cv, dv)
	} else if isAheadMajor(cv, dv) {
		fmt.Fprintln(out, messages.WARNING_DOWNGRADE_VERSION, cv, dv)
	}
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

func formatMajor(l string) string {
	return l[:strings.LastIndex(l, ".")]
}

func formatLtConstraint(c string) string {
	return "< " + c
}

func formatDowngradeConstraint(c string) string {
	return "> " + formatMajor(c)
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
