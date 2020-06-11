package version

import (
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/deployment"

	"github.com/Masterminds/semver"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
)

// ValidateCompatibility print message if astro-cli version is not compatible with platform version
func ValidateCompatibility(client *houston.Client, out io.Writer, cliVer string) error {
	serverCfg, err := deployment.AppConfig(client)
	if err != nil {
		return err
	}
	// Skip check if AppConfig is nil or is cv is empty
	if serverCfg != nil && cliVer != "" {
		return compareVersions(serverCfg.Version, cliVer, out)
	}
	return nil
}

func compareVersions(serverVer, cliVer string, out io.Writer) error {
	if isBehindMajor(serverVer, cliVer) {
		fmt.Fprintf(out, messages.ERROR_NEW_MAJOR_VERSION, cliVer, serverVer)
	} else if isBehindPatch(serverVer, cliVer) {
		fmt.Fprintf(out, messages.WARNING_NEW_PATCH_VERSION, cliVer, serverVer)
	} else if isAheadMajor(serverVer, cliVer) {
		fmt.Fprintf(out, messages.WARNING_DOWNGRADE_VERSION, cliVer, serverVer)
	}
	return nil
}

func isBehindMajor(serverVer, cliVer string) bool {
	fm := formatMajor(serverVer)
	fc := formatLtConstraint(fm)
	maj := getConstraint(fc)
	ver, err := parseVersion(cliVer)
	if err != nil {
		// TODO: add error message
		return false
	}
	return maj.Check(ver)
}

func isBehindPatch(serverVer, cliVer string) bool {
	fc := formatLtConstraint(serverVer)
	patch := getConstraint(fc)
	ver, err := parseVersion(cliVer)
	if err != nil {
		// TODO: add error message
		return false
	}

	return patch.Check(ver)
}

func isAheadMajor(serverVer, cliVer string) bool {
	fc := formatDowngradeConstraint(serverVer)
	ahead := getConstraint(fc)
	ver, err := parseVersion(cliVer)
	if err != nil {
		// TODO: add error message
		return false
	}

	return ahead.Check(ver)
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

func parseVersion(version string) (*semver.Version, error) {
	return semver.NewVersion(version)
}
