package deployment

import (
	"github.com/pkg/browser"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

var openURL = browser.OpenURL

// UI finds a Deployment and the URL of its Airflow UI. Opening it is the
// caller's to do (OpenInBrowser): a run under --output json reads the URL from
// the result instead.
func UI(ws, deploymentID, deploymentName string, astroV1Client astrov1.APIClient) (*UIResult, error) {
	d, err := GetDeployment(ws, deploymentID, deploymentName, ToRead, nil, astroV1Client)
	if err != nil {
		return nil, err
	}
	return &UIResult{DeploymentID: d.Id, Name: d.Name, URL: "https://" + d.WebServerUrl}, nil
}

// OpenInBrowser opens url in the default browser, reporting whether it could.
func OpenInBrowser(url string) bool {
	return openURL(url) == nil
}
