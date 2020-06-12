package version

import (
	"fmt"
	"io"

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
	semVerServer, serverErr := parseVersion(serverVer)
	if serverErr != nil {
		return serverErr
	}

	semVerCli, cliErr := parseVersion(cliVer)
	if cliErr != nil {
		return cliErr
	}

	cliMajor := semVerCli.Major()
	cliMinor := semVerCli.Minor()
	cliPatch := semVerCli.Patch()

	serverMajor := semVerServer.Major()
	serverMinor := semVerServer.Minor()
	serverPatch := semVerServer.Patch()

	if cliMajor < serverMajor {
		return errors.Errorf(messages.ERROR_NEW_MAJOR_VERSION, cliVer, serverVer)
	} else if cliMinor < serverMinor {
		fmt.Fprintf(out, messages.WARNING_NEW_MINOR_VERSION, cliVer, serverVer)
	} else if cliPatch < serverPatch {
		fmt.Fprintf(out, messages.WARNING_NEW_PATCH_VERSION, cliVer, serverVer)
	} else if cliMinor > serverMinor {
		fmt.Fprintf(out, messages.WARNING_DOWNGRADE_VERSION, cliVer, serverVer)
	}

	return nil
}

func parseVersion(version string) (*semver.Version, error) {
	return semver.NewVersion(version)
}
