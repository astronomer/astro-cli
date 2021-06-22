package cmd

import (
	"fmt"
	"github.com/astronomer/astro-cli/pkg/httputil"
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
		switch t := err.(type) {
		default:
			return "", errors.New(fmt.Sprintf("An Unexpected Error occurred: %s", err.Error()))
		case *httputil.Error:
			return "", errors.New(fmt.Sprintf("An error occurred when trying to connect to the houston sever Status Code: %d, Error: %s", t.Status, t.Message))
		}
	}

	if len(defaultImageTag) == 0 {
		defaultImageTag = "2.0.0-buster-onbuild"
		fmt.Fprintf(out, "Initializing Airflow project\nNot connected to Astronomer, pulling Airflow development files from %s\n", defaultImageTag)
	}
	return defaultImageTag, nil
}
