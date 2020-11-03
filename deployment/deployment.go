package deployment

import (
	"fmt"
	"github.com/Masterminds/semver"
	"io"
	"sort"
	"strconv"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/fatih/camelcase"
)

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "TAG", "AIRFLOW VERSION"},
	}
}

// AppConfig returns application config from houston-api
func AppConfig(client *houston.Client) (*houston.AppConfig, error) {
	req := houston.Request{
		Query: houston.AppConfigRequest,
	}
	r, err := req.DoWithClient(client)
	if err != nil {
		return nil, err
	}

	return r.Data.GetAppConfig, nil
}

func checkManualReleaseNames(client *houston.Client) bool {
	req := houston.Request{
		Query: houston.AppConfigRequest,
	}
	r, err := req.DoWithClient(client)
	if err != nil {
		return false
	}

	return r.Data.GetAppConfig.ManualReleaseNames
}

// Create airflow deployment
func Create(label, ws, releaseName, cloudRole, executor, airflowVersion string, client *houston.Client, out io.Writer) error {
	vars := map[string]interface{}{"label": label, "workspaceId": ws, "executor": executor, "cloudRole": cloudRole}

	if releaseName != "" && checkManualReleaseNames(client) {
		vars["releaseName"] = releaseName
	}

	if airflowVersion != "" {
		vars["airflowVersion"] = airflowVersion
	}

	req := houston.Request{
		Query:     houston.DeploymentCreateRequest,
		Variables: vars,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	d := r.Data.CreateDeployment
	tab := newTableOut()
	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.Id, "-", d.AirflowVersion}, false)

	splitted := []string{"Celery", ""}

	if executor != "" {
		// trim executor from console message
		splitted = camelcase.Split(executor)
	}

	var airflowUrl, flowerUrl string
	for _, url := range r.Data.CreateDeployment.Urls {
		if url.Type == "airflow" {
			airflowUrl = url.Url
		}
		if url.Type == "flower" {
			flowerUrl = url.Url
		}
	}

	tab.SuccessMsg =
		fmt.Sprintf("\n Successfully created deployment with %s executor", splitted[0]) +
			". Deployment can be accessed at the following URLs \n" +
			fmt.Sprintf("\n Airflow Dashboard: %s", airflowUrl)

	// The Flower URL is specific to CeleryExecutor only
	if executor == "CeleryExecutor" || executor == "" {
		tab.SuccessMsg += fmt.Sprintf("\n Flower Dashboard: %s", flowerUrl)
	}
	tab.Print(out)

	return nil
}

func Delete(id string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.DeploymentDeleteRequest,
		Variables: map[string]interface{}{"deploymentId": id},
	}

	_, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	// TODO - add back in tab print once houston returns all relevant information
	// tab.AddRow([]string{d.Label, d.ReleaseName, d.Id, d.Workspace.Id}, false)
	// tab.SuccessMsg = "\n Successfully deleted deployment"
	// tab.Print(os.Stdout)
	fmt.Fprintln(out, "\n Successfully deleted deployment")

	return nil
}

// List all airflow deployments
func List(ws string, all bool, client *houston.Client, out io.Writer) error {
	var deployments []houston.Deployment
	var r *houston.Response
	var err error

	req := houston.Request{
		Query: houston.DeploymentsGetRequest,
	}

	if all {
		r, err = req.DoWithClient(client)
		if err != nil {
			return err
		}
	} else {
		req.Variables = map[string]interface{}{"workspaceId": ws}
		r, err = req.DoWithClient(client)
		if err != nil {
			return err
		}
	}

	deployments = r.Data.GetDeployments

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Label > deployments[j].Label })

	tab := newTableOut()

	// Build rows
	for _, d := range deployments {
		if all {
			ws = d.Workspace.Id
		}

		currentTag := d.DeploymentInfo.Current
		if currentTag == "" {
			currentTag = "?"
		}
		tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.Id, currentTag, d.AirflowVersion}, false)
	}

	return tab.Print(out)
}

// Update an airflow deployment
func Update(id, cloudRole string, args map[string]string, client *houston.Client, out io.Writer) error {
	vars := map[string]interface{}{"deploymentId": id, "payload": args, "cloudRole": cloudRole}

	// sync with commander only when we have cloudRole
	if cloudRole != "" {
		vars["sync"] = true
	}

	req := houston.Request{
		Query:     houston.DeploymentUpdateRequest,
		Variables: vars,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	d := r.Data.UpdateDeployment
	tab := newTableOut()
	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.Id, d.AirflowVersion}, false)
	tab.SuccessMsg = "\n Successfully updated deployment"
	tab.Print(out)

	return nil
}

// Upgrade airflow deployment
func AirflowUpgrade(id, desiredAirflowVersion string, client *houston.Client, out io.Writer) error {
	if desiredAirflowVersion == "" {
		selectedVersion, err := getAirflowVersionSelection(id, client, out)
		if err != nil {
			return err
		}
		desiredAirflowVersion = selectedVersion
	}

	vars := map[string]interface{}{"deploymentId": id, "desiredAirflowVersion": desiredAirflowVersion}

	req := houston.Request{
		Query:     houston.UpdateDeploymentAirflowRequest,
		Variables: vars,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}

	d := r.Data.UpdateDeploymentAirflow
	tab := &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "AIRFLOW VERSION"},
	}
	tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.Id, d.AirflowVersion}, false)

	tab.SuccessMsg = fmt.Sprintf("\nThe upgrade from Airflow %s to %s has been started.", d.AirflowVersion, d.DesiredAirflowVersion) +
		"To complete this process, replace the image referenced in your Dockerfile and deploy to Astronomer.\n" +
		"To cancel, run: \n $ astro deployment airflow upgrade --cancel\n"

	tab.Print(out)

	return nil
}

// Upgrade airflow deployment
func AirflowUpgradeCancel(id string, client *houston.Client, out io.Writer) error {
	deployment := getDeployment(id, client)

	if deployment.DesiredAirflowVersion != deployment.AirflowVersion {
		vars := map[string]interface{}{"deploymentId": id, "desiredAirflowVersion": deployment.AirflowVersion}

		req := houston.Request{
			Query:     houston.UpdateDeploymentAirflowRequest,
			Variables: vars,
		}

		_, err := req.DoWithClient(client)
		if err != nil {
			return err
		}

		text := "\nAirflow upgrade process has been successfully canceled. You are using right now %s\n"
		fmt.Fprintf(out, text, deployment.AirflowVersion)
		return nil
	}

	text := "\nNothing to cancel. You are currently using airflow version %s and desired airflow version is %s\n"
	fmt.Fprintf(out, text, deployment.AirflowVersion, deployment.DesiredAirflowVersion)
	return nil
}

func getAirflowVersionSelection(deploymentId string, client *houston.Client, out io.Writer) (string, error) {
	deployment := getDeployment(deploymentId, client)
	currentAirflowVersion, err := semver.NewVersion(deployment.AirflowVersion)
	if err != nil {
		return "", err
	}
	// prepare list of AC airflow versions
	dReq := houston.Request{
		Query: houston.DeploymentInfoRequest,
	}

	resp, err := dReq.DoWithClient(client)
	if err != nil {
		return "", err
	}
	airflowVersions := resp.Data.DeploymentConfig.AirflowVersions

	t := &printutil.Table{
		Padding:        []int{10},
		DynamicPadding: true,
		Header:         []string{"AIRFLOW VERSION"},
	}
	t.GetUserInput = true

	for _, v := range airflowVersions {
		vv, _ := semver.NewVersion(v)
		// false means no colors
		if currentAirflowVersion.LessThan(vv) {
			t.AddRow([]string{v}, false)
		}
	}

	t.Print(out)

	in := input.InputText("\n> ")
	i, err := strconv.ParseInt(
		in,
		10,
		64,
	)
	return airflowVersions[i-1], nil
}


func getDeployment(deploymentId string, client *houston.Client) *houston.Deployment {
	vars := map[string]interface{}{"id": deploymentId}

	req := houston.Request{
		Query:     houston.DeploymentGetRequest,
		Variables: vars,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return nil
	}

	return &r.Data.GetDeployment
}