package version

import (
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/deployment"
	"github.com/pkg/errors"

	"github.com/Masterminds/semver"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
)

// ValidateCompatibility print message if astro-cli version is not compatible with platform version
func ValidateCompatibility(client *houston.Client, out io.Writer, cliVer string, skipVerCheck bool) error {
	if skipVerCheck {
		return nil
	}

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

func compareVersions(serverVer string, cliVer string, out io.Writer) error {
	if isBehindMajor(serverVer, cliVer) {
		return errors.Errorf(messages.ERROR_NEW_MAJOR_VERSION, cliVer, serverVer)
	} else if isBehindMinor(serverVer, cliVer) {
		fmt.Fprintf(out, messages.WARNING_NEW_MINOR_VERSION, cliVer, serverVer)
	} else if isBehindPatch(serverVer, cliVer) {
		fmt.Fprintf(out, messages.WARNING_NEW_PATCH_VERSION, cliVer, serverVer)
	} else if isAheadMinor(serverVer, cliVer) {
		fmt.Fprintf(out, messages.WARNING_DOWNGRADE_VERSION, cliVer, serverVer)
	}
	return nil
}

func isBehindMajor(serverVer string, cliVer string) bool {
	fMaj := formatMajor(serverVer)
	return checkBehindVersion(fMaj, cliVer)
}

func isBehindMinor(serverVer string, cliVer string) bool {
	fMin := formatMinor(serverVer)
	return checkBehindVersion(fMin, cliVer)
}

func isBehindPatch(serverVer string, cliVer string) bool {
	return checkBehindVersion(serverVer, cliVer)
}

func checkBehindVersion(serverVer string, cliVer string) bool {
	fCon := formatLtConstraint(serverVer)
	return checkFormattedConstraint(fCon, cliVer)
}

func isAheadMinor(serverVer string, cliVer string) bool {
	fMaj := formatMinor(serverVer)
	fCon := formatGtConstraint(fMaj)
	return checkFormattedConstraint(fCon, cliVer)
}

func checkFormattedConstraint(fCon string, cliVer string) bool {
	con := getConstraint(fCon)
	ver, err := parseVersion(cliVer)
	if err != nil {
		// TODO: add error message
		return false
	}
	return con.Check(ver)
}

func formatMajor(l string) string {
	return l[:strings.Index(l, ".")]
}

func formatMinor(l string) string {
	return l[:strings.LastIndex(l, ".")]
}

func formatLtConstraint(c string) string {
	return "< " + c
}

func formatGtConstraint(c string) string {
	return "> " + c
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
