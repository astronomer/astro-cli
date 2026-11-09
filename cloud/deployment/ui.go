package deployment

import (
	"fmt"
	"io"

	"github.com/pkg/browser"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

var openURL = browser.OpenURL

// OpenUI opens a Deployment's Airflow UI in the default browser.
func OpenUI(ws, deploymentID, deploymentName string, out io.Writer, astroV1Client astrov1.APIClient) error {
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, nil, astroV1Client)
	if err != nil {
		return err
	}
	if currentDeployment.Id == "" {
		fmt.Fprintf(out, "%s %s\n", NoDeploymentInWSMsg, ansi.Bold(ws))
		return nil
	}

	url := "https://" + currentDeployment.WebServerUrl
	err = openURL(url)
	if err != nil {
		fmt.Fprintf(out, "Unable to open the Airflow UI, please visit the following link: %s\n", url)
		return nil
	}
	fmt.Fprintf(out, "Opened the Airflow UI for Deployment %s: %s\n", ansi.Bold(currentDeployment.Name), url)
	return nil
}
