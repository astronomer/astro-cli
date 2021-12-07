package cmd

import (
	"fmt"
	"io"
	"strings"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"

	"github.com/pkg/errors"
)

func prepareDefaultAirflowImageTag(airflowVersion string, httpClient *airflowversions.Client, houstonClient *houston.Client, out io.Writer) (string, error) {
	r := houston.Request{
		Query: houston.DeploymentInfoRequest,
	}

	wsResp, err := r.DoWithClient(houstonClient)

	if err == nil {
		acceptableAirflowVersions := wsResp.Data.DeploymentConfig.AirflowVersions
		if airflowVersion != "" && !acceptableVersion(airflowVersion, acceptableAirflowVersions) {
			return "", errors.Errorf(messages.ErrInvalidAirflowVersion, strings.Join(acceptableAirflowVersions, ", "))
		}
	} else if airflowVersion != "" {
		switch t := err; t {
		case houston.ErrVerboseInaptPermissions:
			return "", errors.New("the --airflow-version flag is not supported if you're not authenticated to Astronomer. Please authenticate and try again")
		default:
			return "", err
		}
	}

	defaultImageTag, _ := airflowversions.GetDefaultImageTag(httpClient, airflowVersion)

	if defaultImageTag == "" {
		defaultImageTag = "2.0.0-buster-onbuild"
		fmt.Fprintf(out, "Initializing Airflow project, pulling Airflow development files from %s\n", defaultImageTag)
	}
	return defaultImageTag, nil
}
