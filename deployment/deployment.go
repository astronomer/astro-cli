package deployment

import (
	"fmt"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/httputil"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

func Create(title, ws string) error {
	deployment, err := api.CreateDeployment(title, ws)
	if err != nil {
		return err
	}

	fmt.Printf(messages.HOUSTON_DEPLOYMENT_CREATE_SUCCESS, deployment.Id)

	fmt.Printf("\n"+messages.EE_LINK_AIRFLOW+"\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())
	fmt.Printf(messages.EE_LINK_FLOWER+"\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())
	fmt.Printf(messages.EE_LINK_GRAFANA+"\n", deployment.ReleaseName, config.CFG.CloudDomain.GetString())

	return nil
}

func Delete(uuid string) error {
	resp, err := api.DeleteDeployment(uuid)
	if err != nil {
		return err
	}

	fmt.Printf(messages.HOUSTON_DEPLOYMENT_DELETE_SUCCESS, resp.Id)

	return nil
}

// List all airflow deployments
func List(ws string) error {
	deployments, err := api.FetchDeployments(ws)
	if err != nil {
		return err
	}

	for _, d := range deployments {
		rowTmp := "Title: %s\nId: %s\nRelease: %s\nVersion: %s\n\n"
		fmt.Printf(rowTmp, d.Title, d.Id, d.ReleaseName, d.Version)
	}
	return nil
}
