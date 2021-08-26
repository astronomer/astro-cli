package version

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/logger"
	"github.com/pkg/errors"

	"github.com/Masterminds/semver"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
)

var newLogger = logger.NewLogger()

// ValidateCompatibility print message if astro-cli version is not compatible with platform version
func ValidateCompatibility(client *houston.Client, out io.Writer, cliVer string, skipVerCheck bool) error {
	if skipVerCheck {
		return nil
	}
	newLogger.Debug("checking if astro-cli version is not compatible with platform")

	serverCfg, err := deployment.AppVersion(client)
	if err != nil {
		return err
	}
	// Skip check if AppConfig is nil or is cv is empty
	if serverCfg != nil && cliVer != "" {
		return compareVersions(serverCfg.Version, cliVer, out)
	}

	return nil
}

// compareVersions print warning message if astro-cli has a variation in the minor version. Errors if major version is behind.
func compareVersions(compareVer string, currentVer string, out io.Writer) error {
	semCompareVer, err := parseVersion(compareVer)
	if err != nil {
		return err
	}

	semCurrVer, err := parseVersion(currentVer)
	if err != nil {
		return err
	}

	currMajor := semCurrVer.Major()
	currMinor := semCurrVer.Minor()

	compareMajor := semCompareVer.Major()
	compareMinor := semCompareVer.Minor()

	if currMajor < compareMajor {
		return errors.Errorf(messages.ERROR_NEW_MAJOR_VERSION, currentVer, compareVer)
	} else if currMinor < compareMinor {
		fmt.Fprintf(out, messages.WARNING_NEW_MINOR_VERSION, currentVer, compareVer)
	} else if currMinor > compareMinor {
		fmt.Fprintf(out, messages.WARNING_DOWNGRADE_VERSION, currentVer, compareVer)
	}

	return nil
}

func parseVersion(version string) (*semver.Version, error) {
	return semver.NewVersion(version)
}
