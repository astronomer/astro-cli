package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
)

var errAirflowVersionNotSupported = errors.New("the --airflow-version flag is not supported if you're not authenticated to Astronomer. Please authenticate and try again")

func prepareDefaultAirflowImageTag(airflowVersion, userRuntimeVersion string, httpClient *airflowversions.Client, houstonClient houston.ClientInterface, out io.Writer) (string, error) {
	deploymentConfig, err := houstonClient.GetDeploymentConfig()
	if err == nil {
		acceptableAirflowVersions := deploymentConfig.AirflowVersions
		if airflowVersion != "" && !acceptableVersion(airflowVersion, acceptableAirflowVersions) {
			return "", fmt.Errorf(messages.ErrInvalidAirflowVersion, strings.Join(acceptableAirflowVersions, ", ")) //nolint:goerr113
		}
	} else if airflowVersion != "" {
		switch t := err; t {
		case houston.ErrVerboseInaptPermissions:
			return "", errAirflowVersionNotSupported
		default:
			return "", err
		}
	}

	defaultImageTag, _ := airflowversions.GetDefaultImageTag(httpClient, airflowVersion, userRuntimeVersion)

	if defaultImageTag == "" {
		if useAstronomerCertified {
			defaultImageTag = "2.0.0-buster-onbuild"
		} else {
			defaultImageTag = "3.0.0"
		}
		fmt.Fprintf(out, "Initializing Airflow project, pulling Airflow development files from %s\n", defaultImageTag)
	}
	return defaultImageTag, nil
}
