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
	defaultImageTag, _ := airflowversions.GetDefaultImageTag(httpClient, "")

	r := houston.Request{
		Query: houston.DeploymentInfoRequest,
	}

	wsResp, err := r.DoWithClient(houstonClient)

	if err == nil {
		acceptableAirflowVersions := wsResp.Data.DeploymentConfig.AirflowVersions
		if airflowVersion != "" && !acceptableVersion(airflowVersion, acceptableAirflowVersions) {
			return "", errors.Errorf(messages.ERROR_INVALID_AIRFLOW_VERSION, strings.Join(acceptableAirflowVersions, ", "))
		}
		if airflowVersion == "" {
			defaultImageTag = ""
		} else {
			defaultImageTag = fmt.Sprintf("%s-buster-onbuild", airflowVersion)
		}
	} else if airflowVersion != "" {
		return "", err
	}

	if len(defaultImageTag) == 0 {
		defaultImageTag = "2.0.0-buster-onbuild"
		fmt.Fprintf(out, "Initializing Airflow project\nNot connected to Astronomer, pulling Airflow development files from %s\n", defaultImageTag)
	}
	return defaultImageTag, nil
}
