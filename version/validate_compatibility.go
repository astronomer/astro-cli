package version

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"

	semver "github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ValidateCompatibility print message if astro-cli version is not compatible with platform version
func ValidateCompatibility(client *houston.Client, out io.Writer, cliVer string, skipVerCheck bool) error {
	if skipVerCheck {
		return nil
	}
	logrus.Debug("checking if astro-cli version is not compatible with platform")

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
func compareVersions(compareVer, currentVer string, out io.Writer) error {
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

	switch {
	case currMajor < compareMajor:
		return errors.Errorf(messages.ErrNewMajorVersion, currentVer, compareVer)
	case currMinor < compareMinor:
		fmt.Fprintf(out, messages.WarningNewMinorVersion, currentVer, compareVer)
	case currMinor > compareMinor:
		fmt.Fprintf(out, messages.WarningDowngradeVersion, currentVer, compareVer)
	}

	return nil
}

func parseVersion(version string) (*semver.Version, error) {
	return semver.NewVersion(version)
}
