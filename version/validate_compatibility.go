package version

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"

	semver "github.com/Masterminds/semver/v3"
	"github.com/sirupsen/logrus"
)

// ValidateCompatibility print message if astro-cli version is not compatible with platform version
func ValidateCompatibility(client houston.ClientInterface, out io.Writer, cliVer string, skipVerCheck bool) error {
	if skipVerCheck {
		return nil
	}
	logrus.Debug("checking if astro-cli version is not compatible with platform")

	serverCfg, err := client.GetAppConfig()
	if err != nil {
		var e *houston.ErrFieldsNotAvailable
		if errors.As(err, &e) {
			logrus.Debugln(e.BaseError)
		}
		return ErrVersionMismatch{}
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
		return fmt.Errorf(messages.ErrNewMajorVersion, currentVer, compareVer) //nolint:goerr113
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
