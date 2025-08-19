package deployment

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/logger"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/settings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/fatih/camelcase"
	giturls "github.com/whilp/git-urls"
)

var (
	ErrKubernetesNamespaceNotAvailable = errors.New("no kubernetes namespaces are available")
	ErrNumberOutOfRange                = errors.New("number is out of available range")
	ErrMajorAirflowVersionUpgrade      = fmt.Errorf("Airflow 2.0 has breaking changes. To upgrade to Airflow 2.0, upgrade to %s first and make sure your DAGs and configs are 2.0 compatible", minAirflowVersion) //nolint:golint,stylecheck
	ErrKubernetesNamespaceNotSpecified = errors.New("no kubernetes namespaces specified")
	errInvalidSSHKeyPath               = errors.New("wrong path specified, no file exists for ssh key")
	errInvalidKnownHostsPath           = errors.New("wrong path specified, no file exists for known hosts")
	errHostNotPresent                  = errors.New("git repository host not present in known hosts file")

	ErrInvalidDeploymentKey = errors.New("invalid Deployment selected")

	errDeploymentNotOnRuntime     = errors.New("deployment is not using Runtime image, please migrate to Runtime image via `astro deployment runtime migrate` before trying to upgrade Runtime version")
	errDeploymentNotOnAirflow     = errors.New("deployment is not using Airflow image, please make sure deployment is using Airflow image before trying to upgrade Airflow version")
	errDeploymentAlreadyOnRuntime = errors.New("deployment is already using runtime image")
	errRuntimeUpdateFailed        = errors.New("failed to update the deployment runtime version")
	errInvalidAirflowVersion      = errors.New("invalid Airflow version to migrate the deployment to Runtime, please upgrade the deployment to 2.2.4 Airflow version before trying to migrate to Runtime image")
)

const (
	minAirflowVersion = "1.10.14"

	runtimeImageType   = "Runtime"
	certifiedImageType = "Astronomer-Certified"
)

type ErrParsingInt struct {
	in string
}

func (e ErrParsingInt) Error() string {
	return fmt.Sprintf("cannot parse %s to int", e.in)
}

type ErrInvalidAirflowVersion struct {
	desiredVersion string
	currentVersion *semver.Version
}

func (e ErrInvalidAirflowVersion) Error() string {
	return fmt.Sprintf("Error: You tried to set --desired-airflow-version to %s, but this Airflow Deployment "+
		"is already running %s. Please indicate a higher version of Airflow and try again.", e.desiredVersion, e.currentVersion)
}

type ErrInvalidRuntimeVersion struct {
	desiredVersion string
	currentVersion *semver.Version
}

func (e ErrInvalidRuntimeVersion) Error() string {
	return fmt.Sprintf("Error: You tried to set --desired-runtime-version to %s, but this Runtime Deployment "+
		"is already running %s. Please indicate a higher version of Runtime and try again.", e.desiredVersion, e.currentVersion)
}

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "TAG", "IMAGE VERSION"},
	}
}

func checkManualReleaseNames(client houston.ClientInterface) bool {
	logger.Debug("Checking checkManualReleaseNames through appConfig from houston-api")

	config, err := houston.Call(client.GetAppConfig)(nil)
	if err != nil {
		return false
	}

	return config.ManualReleaseNames
}

// CheckNFSMountDagDeployment returns true when we can set custom NFS location for dags
func CheckNFSMountDagDeployment(client houston.ClientInterface) bool {
	logger.Debug("Checking checkNFSMountDagDeployment through appConfig from houston-api")

	config, err := houston.Call(client.GetAppConfig)(nil)
	if err != nil {
		return false
	}

	return config.Flags.NfsMountDagDeployment
}

func CheckHardDeleteDeployment(client houston.ClientInterface) bool {
	logger.Debug("Checking for hard delete deployment flag")
	config, err := houston.Call(client.GetAppConfig)(nil)
	if err != nil {
		return false
	}
	return config.Flags.HardDeleteDeployment
}

func CheckPreCreateNamespaceDeployment(client houston.ClientInterface) bool {
	logger.Debug("Checking for pre created deployment flag")
	config, err := houston.Call(client.GetAppConfig)(nil)
	if err != nil {
		return false
	}
	return config.Flags.ManualNamespaceNames
}

func CheckNamespaceFreeFormEntryDeployment(client houston.ClientInterface) bool {
	config, err := houston.Call(client.GetAppConfig)(nil)
	if err != nil {
		return false
	}
	return config.Flags.NamespaceFreeFormEntry
}

func CheckTriggererEnabled(client houston.ClientInterface) bool {
	logger.Debug("Checking for triggerer flag")
	config, err := houston.Call(client.GetAppConfig)(nil)
	if err != nil {
		return false
	}
	return config.Flags.TriggererEnabled
}

func addTriggererReplicasArg(vars map[string]interface{}, client houston.ClientInterface, triggererReplicas int) {
	if CheckTriggererEnabled(client) && triggererReplicas != -1 {
		vars["triggererReplicas"] = triggererReplicas
	}
}

// Create airflow deployment
func Create(req *CreateDeploymentRequest, client houston.ClientInterface, out io.Writer) error {
	vars := map[string]interface{}{"label": req.Label, "workspaceId": req.WS, "executor": req.Executor, "cloudRole": req.CloudRole}

	if req.ClusterID != "" {
		vars["clusterId"] = req.ClusterID
	}

	if CheckPreCreateNamespaceDeployment(client) {
		namespace, err := getDeploymentSelectionNamespaces(client, out, req.ClusterID)
		if err != nil {
			return err
		}
		vars["namespace"] = namespace
	}

	if CheckNamespaceFreeFormEntryDeployment(client) {
		namespace, err := getDeploymentNamespaceName()
		if err != nil {
			return err
		}
		vars["namespace"] = namespace
	}

	if req.ReleaseName != "" && checkManualReleaseNames(client) {
		vars["releaseName"] = req.ReleaseName
	}

	if req.AirflowVersion != "" {
		vars["airflowVersion"] = req.AirflowVersion
	} else if req.RuntimeVersion != "" {
		vars["runtimeVersion"] = req.RuntimeVersion
	}

	err := addDagDeploymentArgs(vars, req.DAGDeploymentType, req.NFSLocation, req.SSHKey, req.KnownHosts, req.GitRepoURL, req.GitRevision, req.GitBranchName, req.GitDAGDir, req.GitSyncInterval)
	if err != nil {
		return err
	}

	addTriggererReplicasArg(vars, client, req.TriggererReplicas)

	d, err := houston.Call(client.CreateDeployment)(vars)
	if err != nil {
		return err
	}

	tab := newTableOut()
	var resp []string
	if d.AirflowVersion != "" {
		resp = []string{d.Label, d.ReleaseName, d.Version, d.ID, "-", fmt.Sprintf("%s-%s", certifiedImageType, d.AirflowVersion)}
	} else {
		resp = []string{d.Label, d.ReleaseName, d.Version, d.ID, "-", fmt.Sprintf("%s-%s", runtimeImageType, d.RuntimeVersion)}
	}
	tab.AddRow(resp, false)

	splitted := []string{"Celery", ""}

	if req.Executor != "" {
		// trim executor from console message
		splitted = camelcase.Split(req.Executor)
	}

	var airflowURL, flowerURL string
	for _, url := range d.Urls {
		if url.Type == "airflow" {
			airflowURL = url.URL
		}
		if url.Type == "flower" {
			flowerURL = url.URL
		}
	}

	tab.SuccessMsg = fmt.Sprintf("\n Successfully created deployment with %s executor", splitted[0]) +
		". Deployment can be accessed at the following URLs \n" +
		fmt.Sprintf("\n Airflow Dashboard: %s", airflowURL)

	// The Flower URL is specific to CeleryExecutor only
	if req.Executor == houston.CeleryExecutorType || req.Executor == "" {
		tab.SuccessMsg += fmt.Sprintf("\n Flower Dashboard: %s", flowerURL)
	}
	tab.Print(out)

	return nil
}

func Delete(id string, hardDelete bool, client houston.ClientInterface, out io.Writer) error {
	_, err := houston.Call(client.DeleteDeployment)(houston.DeleteDeploymentRequest{DeploymentID: id, HardDelete: hardDelete})
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

// list all available namespaces
func getDeploymentSelectionNamespaces(client houston.ClientInterface, out io.Writer, clusterID string) (string, error) {
	tab := &printutil.Table{
		Padding:        []int{30},
		DynamicPadding: true,
		Header:         []string{"AVAILABLE KUBERNETES NAMESPACES"},
	}

	logger.Debug("checking namespaces available for platform")
	tab.GetUserInput = true

	names, err := houston.Call(client.GetAvailableNamespaces)(map[string]interface{}{"clusterID": clusterID})
	if err != nil {
		return "", err
	}

	if len(names) == 0 {
		return "", ErrKubernetesNamespaceNotAvailable
	}

	for _, namespace := range names {
		name := namespace.Name

		tab.AddRow([]string{name}, false)
	}

	tab.Print(out)

	in := input.Text("\n> ")
	i, err := strconv.ParseInt(in, 10, 64) //nolint:mnd
	if err != nil {
		return "", ErrParsingInt{in: in}
	}
	if i > int64(len(names)) {
		return "", ErrNumberOutOfRange
	}
	return names[i-1].Name, nil
}

func getDeploymentNamespaceName() (string, error) {
	namespaceName := input.Text("\nKubernetes Namespace Name: ")
	noSpaceString := strings.ReplaceAll(namespaceName, " ", "")
	if noSpaceString == "" {
		return "", ErrKubernetesNamespaceNotSpecified
	}
	return namespaceName, nil
}

func getDeploymentsFromHouston(ws string, all bool, client houston.ClientInterface, clusterID string) ([]houston.Deployment, error) {
	if all {
		return houston.Call(client.ListPaginatedDeployments)(houston.PaginatedDeploymentsRequest{
			Take:      -1,
			ClusterID: clusterID,
		})
	}
	listDeploymentRequest := houston.ListDeploymentsRequest{}
	listDeploymentRequest.WorkspaceID = ws
	return houston.Call(client.ListDeployments)(listDeploymentRequest)
}

// List all airflow deployments
func List(ws string, all bool, client houston.ClientInterface, out io.Writer, clusterID string) error {
	deployments, err := getDeploymentsFromHouston(ws, all, client, clusterID)
	if err != nil {
		return err
	}

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Label > deployments[j].Label })

	tab := newTableOut()

	// Build rows
	for i := range deployments {
		d := deployments[i]

		currentTag := d.DeploymentInfo.Current
		if currentTag == "" {
			currentTag = "?"
		}
		var resp []string
		if d.RuntimeVersion != "" {
			resp = []string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, currentTag, fmt.Sprintf("%s-%s", runtimeImageType, d.RuntimeVersion)}
		} else {
			resp = []string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, currentTag, fmt.Sprintf("%s-%s", certifiedImageType, d.AirflowVersion)}
		}
		tab.AddRow(resp, false)
	}

	return tab.Print(out)
}

// Update an airflow deployment
func Update(id, cloudRole string, args map[string]string, dagDeploymentType, nfsLocation, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, sshKey, knownHosts, executor string, gitSyncInterval, triggererReplicas int, client houston.ClientInterface, out io.Writer) error {
	vars := map[string]interface{}{"deploymentId": id, "payload": args, "cloudRole": cloudRole}

	// sync with commander only when we have cloudRole
	if cloudRole != "" {
		vars["sync"] = true
	}

	if executor != "" {
		vars["executor"] = executor
	}

	// adds dag deployment args to the vars map
	err := addDagDeploymentArgs(vars, dagDeploymentType, nfsLocation, sshKey, knownHosts, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, gitSyncInterval)
	if err != nil {
		return err
	}

	if CheckTriggererEnabled(client) && triggererReplicas != -1 {
		vars["triggererReplicas"] = triggererReplicas
	}

	d, err := houston.Call(client.UpdateDeployment)(vars)
	if err != nil {
		return err
	}

	tab := newTableOut()
	currentTag := d.DeploymentInfo.Current
	if currentTag == "" {
		currentTag = "?"
	}
	var resp []string
	if d.AirflowVersion != "" {
		resp = []string{d.Label, d.ReleaseName, d.Version, d.ID, currentTag, fmt.Sprintf("%s-%s", certifiedImageType, d.AirflowVersion)}
	} else {
		resp = []string{d.Label, d.ReleaseName, d.Version, d.ID, currentTag, fmt.Sprintf("%s-%s", runtimeImageType, d.RuntimeVersion)}
	}
	tab.AddRow(resp, false)
	tab.SuccessMsg = "\n Successfully updated deployment"
	tab.Print(out)

	return nil
}

// Upgrade airflow deployment
func AirflowUpgrade(id, desiredAirflowVersion string, client houston.ClientInterface, out io.Writer) error {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return err
	}

	if deployment.RuntimeVersion != "" {
		return errDeploymentNotOnAirflow
	}

	if desiredAirflowVersion == "" {
		selectedVersion, err := getAirflowVersionSelection(deployment.AirflowVersion, client, out)
		if err != nil {
			return err
		}
		desiredAirflowVersion = selectedVersion
	}
	err = meetsAirflowUpgradeReqs(deployment.AirflowVersion, desiredAirflowVersion)
	if err != nil {
		return err
	}

	vars := map[string]interface{}{"deploymentId": id, "desiredAirflowVersion": desiredAirflowVersion}

	d, err := houston.Call(client.UpdateDeploymentAirflow)(vars)
	if err != nil {
		return err
	}

	tab := &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "IMAGE VERSION"},
	}
	tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, fmt.Sprintf("%s-%s", certifiedImageType, d.DesiredAirflowVersion)}, false)

	tab.SuccessMsg = fmt.Sprintf("\nThe upgrade from Airflow %s to %s has been started. ", d.AirflowVersion, d.DesiredAirflowVersion) +
		fmt.Sprintf("To complete this process, add an Airflow %s image to your Dockerfile and deploy to Astronomer.\n", d.DesiredAirflowVersion) +
		"To cancel, run: \n $ astro deployment airflow upgrade --cancel\n"

	tab.Print(out)

	return nil
}

// Upgrade airflow deployment
func AirflowUpgradeCancel(id string, client houston.ClientInterface, out io.Writer) error {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return err
	}

	if deployment.DesiredAirflowVersion != deployment.AirflowVersion {
		vars := map[string]interface{}{"deploymentId": id, "desiredAirflowVersion": deployment.AirflowVersion}

		_, err := houston.Call(client.UpdateDeploymentAirflow)(vars)
		if err != nil {
			return err
		}

		text := "\nAirflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow %s.\n"
		fmt.Fprintf(out, text, deployment.AirflowVersion)
		return nil
	}

	text := "\nNothing to cancel. You are currently running Airflow %s and you have not indicated that you want to upgrade."
	fmt.Fprintf(out, text, deployment.AirflowVersion)
	return nil
}

// RuntimeUpgrade is to upgrade a deployment to newer runtime version
func RuntimeUpgrade(id, desiredRuntimeVersion string, client houston.ClientInterface, out io.Writer) error {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return err
	}

	if deployment.RuntimeVersion == "" && deployment.AirflowVersion != "" {
		return errDeploymentNotOnRuntime
	}

	if desiredRuntimeVersion == "" {
		selectedVersion, err := getRuntimeVersionSelection(deployment.RuntimeVersion, deployment.RuntimeAirflowVersion, deployment.ClusterID, client, out)
		if err != nil {
			return err
		}
		desiredRuntimeVersion = selectedVersion
	}
	err = meetsRuntimeUpgradeReqs(deployment.RuntimeVersion, desiredRuntimeVersion)
	if err != nil {
		return err
	}

	vars := map[string]interface{}{"deploymentUuid": id, "desiredRuntimeVersion": desiredRuntimeVersion}

	d, err := houston.Call(client.UpdateDeploymentRuntime)(vars)
	if err != nil {
		return err
	} else if d == nil {
		return errRuntimeUpdateFailed
	}

	runtimeVersion := fmt.Sprintf("%s-%s", runtimeImageType, d.DesiredRuntimeVersion)
	tab := &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "IMAGE VERSION"},
	}
	tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, runtimeVersion}, false)

	tab.SuccessMsg = fmt.Sprintf("\nThe upgrade from Runtime %s to %s has been started. ", d.RuntimeVersion, desiredRuntimeVersion) +
		fmt.Sprintf("To complete this process, add an Runtime %s image to your Dockerfile and deploy to Astronomer.\n", desiredRuntimeVersion) +
		"To cancel, run: \n $ astro deployment runtime upgrade --cancel\n"

	tab.Print(out)

	return nil
}

// RuntimeUpgradeCancel is to cancel an upgrade operation for a deployment
func RuntimeUpgradeCancel(id string, client houston.ClientInterface, out io.Writer) error { //nolint:dupl
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return err
	}

	if deployment.DesiredRuntimeVersion != deployment.RuntimeVersion {
		vars := map[string]interface{}{"deploymentUuid": id}

		_, err := houston.Call(client.CancelUpdateDeploymentRuntime)(vars)
		if err != nil {
			return err
		}

		text := "\nRuntime upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Runtime %s.\n"
		fmt.Fprintf(out, text, deployment.RuntimeVersion)
		return nil
	}

	text := "\nNothing to cancel. You are currently running Runtime %s and you have not indicated that you want to upgrade."
	fmt.Fprintf(out, text, deployment.RuntimeVersion)
	return nil
}

// RuntimeMigrate is to migrate a deployment from using airflow version to runtime version
func RuntimeMigrate(deploymentID string, client houston.ClientInterface, out io.Writer) error {
	deployment, err := houston.Call(client.GetDeployment)(deploymentID)
	if err != nil {
		return err
	}

	if deployment.AirflowVersion == "" || deployment.RuntimeVersion != "" {
		return errDeploymentAlreadyOnRuntime
	}

	vars := make(map[string]interface{})
	vars["airflowVersion"] = deployment.AirflowVersion
	vars["clusterId"] = deployment.ClusterID
	runtimeReleases, err := houston.Call(client.GetRuntimeReleases)(vars)
	if err != nil {
		return err
	}

	var latestRuntimeRelease *semver.Version
	for idx := range runtimeReleases {
		runtimeVersion, _ := semver.NewVersion(runtimeReleases[idx].Version)
		if latestRuntimeRelease == nil {
			latestRuntimeRelease = runtimeVersion
		} else if runtimeVersion != nil && !latestRuntimeRelease.GreaterThan(runtimeVersion) {
			latestRuntimeRelease = runtimeVersion
		}
	}

	if latestRuntimeRelease == nil {
		return errInvalidAirflowVersion
	}
	desiredRuntimeVersion := latestRuntimeRelease.String()

	vars = map[string]interface{}{"deploymentUuid": deploymentID, "desiredRuntimeVersion": desiredRuntimeVersion}
	resp, err := houston.Call(client.UpdateDeploymentRuntime)(vars)
	if err != nil {
		return err
	} else if resp == nil {
		return errRuntimeUpdateFailed
	}

	tab := &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "IMAGE VERSION"},
	}
	tab.AddRow([]string{resp.Label, resp.ReleaseName, "v" + resp.Version, resp.ID, fmt.Sprintf("%s-%s", runtimeImageType, desiredRuntimeVersion)}, false)

	tab.SuccessMsg = fmt.Sprintf("\nThe migration from Airflow %s image to Runtime %s has been started. ", deployment.AirflowVersion, desiredRuntimeVersion) +
		fmt.Sprintf("To complete this process, add an Runtime %s image to your Dockerfile and deploy to Astronomer.\n", desiredRuntimeVersion) +
		"To cancel, run: \n $ astro deployment runtime migrate --cancel\n"

	tab.Print(out)

	return nil
}

// RuntimeMigrateCancel is to cancel migration operation for a deployment
func RuntimeMigrateCancel(id string, client houston.ClientInterface, out io.Writer) error {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return err
	}

	if deployment.RuntimeVersion == "" && deployment.DesiredRuntimeVersion != "" && deployment.AirflowVersion != "" {
		vars := map[string]interface{}{"deploymentUuid": id}
		_, err := houston.Call(client.CancelUpdateDeploymentRuntime)(vars)
		if err != nil {
			return err
		}

		text := "\nRuntime migrate process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow %s.\n"
		fmt.Fprintf(out, text, deployment.AirflowVersion)
		return nil
	}

	text := "\nNothing to cancel. You are already running Runtime %s and you have either not indicated that you want to migrate or migration has been completed."
	fmt.Fprintf(out, text, deployment.RuntimeVersion)
	return nil
}

func getAirflowVersionSelection(airflowVersion string, client houston.ClientInterface, out io.Writer) (string, error) {
	currentAirflowVersion, err := semver.NewVersion(airflowVersion)
	if err != nil {
		return "", err
	}
	// prepare list of AC airflow versions
	config, err := houston.Call(client.GetDeploymentConfig)(nil)
	if err != nil {
		return "", err
	}
	airflowVersions := config.AirflowVersions

	t := &printutil.Table{
		Padding:        []int{10},
		DynamicPadding: true,
		Header:         []string{"AIRFLOW VERSION"},
	}
	t.GetUserInput = true

	var filteredVersions []string

	for _, v := range airflowVersions {
		vv, _ := semver.NewVersion(v)
		// false means no colors
		if currentAirflowVersion.LessThan(vv) {
			filteredVersions = append(filteredVersions, v)
			t.AddRow([]string{fmt.Sprintf("%s-%s", certifiedImageType, v)}, false)
		}
	}

	t.Print(out)

	in := input.Text("\n> ")
	i, err := strconv.ParseInt(in, 10, 64)
	if err != nil {
		return "", err
	}
	return filteredVersions[i-1], nil
}

func getRuntimeVersionSelection(runtimeVersion, airflowVersion, clusterID string, client houston.ClientInterface, out io.Writer) (string, error) {
	currentRuntimeVersion, err := semver.NewVersion(runtimeVersion)
	if err != nil {
		return "", err
	}
	currentAirflowVersion, err := semver.NewVersion(airflowVersion)
	if err != nil {
		return "", err
	}

	// prepare list of AC airflow versions
	vars := make(map[string]interface{})
	vars["clusterId"] = clusterID
	runtimeVersions, err := houston.Call(client.GetRuntimeReleases)(vars)
	if err != nil {
		return "", err
	}

	t := &printutil.Table{
		Padding:        []int{10},
		DynamicPadding: true,
		Header:         []string{"RUNTIME VERSION"},
	}
	t.GetUserInput = true

	var filteredVersions []string

	for _, v := range runtimeVersions {
		runtimeVersion, err := semver.NewVersion(v.Version)
		if err != nil {
			continue
		}
		airflowVersion, err := semver.NewVersion(v.AirflowVersion)
		if err != nil {
			continue
		}
		if currentRuntimeVersion.LessThan(runtimeVersion) && !currentAirflowVersion.GreaterThan(airflowVersion) {
			filteredVersions = append(filteredVersions, v.Version)
			t.AddRow([]string{fmt.Sprintf("%s-%s", runtimeImageType, v.Version)}, false)
		}
	}

	t.Print(out)

	in := input.Text("\n> ")
	i, err := strconv.ParseInt(in, 10, 64) //nolint:mnd
	if err != nil {
		return "", err
	}
	return filteredVersions[i-1], nil
}

func meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion string) error {
	upgradeVersion := strconv.FormatUint(settings.AirflowVersionTwo, 10) //nolint:mnd
	minRequiredVersion := minAirflowVersion
	airflowUpgradeVersion, err := semver.NewVersion(upgradeVersion)
	if err != nil {
		return err
	}

	desiredVersion, err := semver.NewVersion(desiredAirflowVersion)
	if err != nil {
		return err
	}

	currentVersion, err := semver.NewVersion(airflowVersion)
	if err != nil {
		return err
	}

	if currentVersion.Compare(desiredVersion) == 0 {
		return ErrInvalidAirflowVersion{desiredVersion: desiredAirflowVersion, currentVersion: currentVersion}
	}

	if airflowUpgradeVersion.Compare(desiredVersion) < 1 {
		minUpgrade, err := semver.NewVersion(minRequiredVersion)
		if err != nil {
			return err
		}

		if currentVersion.Compare(minUpgrade) < 0 {
			return ErrMajorAirflowVersionUpgrade
		}
	}

	return nil
}

func meetsRuntimeUpgradeReqs(runtimeVersion, desiredRuntimeVersion string) error {
	desiredVersion, err := semver.NewVersion(desiredRuntimeVersion)
	if err != nil {
		return err
	}

	currentVersion, err := semver.NewVersion(runtimeVersion)
	if err != nil {
		return err
	}

	if currentVersion.Compare(desiredVersion) == 0 {
		return ErrInvalidRuntimeVersion{desiredVersion: desiredRuntimeVersion, currentVersion: currentVersion}
	}

	return nil
}

// addDagDeploymentArgs adds dag deployment argument to houston request map
func addDagDeploymentArgs(vars map[string]interface{}, dagDeploymentType, nfsLocation, sshKey, knownHosts, gitRepoURL, gitRevision, gitBranchName, gitDAGDir string, gitSyncInterval int) error {
	dagDeploymentConfig := map[string]interface{}{}
	if dagDeploymentType != "" {
		dagDeploymentConfig["type"] = dagDeploymentType
	}

	if dagDeploymentType == houston.VolumeDeploymentType && nfsLocation != "" {
		dagDeploymentConfig["nfsLocation"] = nfsLocation
	}

	if dagDeploymentType == houston.GitSyncDeploymentType {
		if sshKey != "" {
			sshPubKey, err := readSSHKeyFile(sshKey)
			if err != nil {
				return err
			}
			dagDeploymentConfig["sshKey"] = sshPubKey

			if knownHosts != "" {
				repoHost, err := getURLHost(gitRepoURL)
				if err != nil {
					return err
				}
				knownHostsVal, err := readKnownHostsFile(knownHosts, repoHost)
				if err != nil {
					return err
				}
				dagDeploymentConfig["knownHosts"] = knownHostsVal
			}
		}
		if gitRevision != "" {
			dagDeploymentConfig["rev"] = gitRevision
		}
		if gitRepoURL != "" {
			dagDeploymentConfig["repositoryUrl"] = gitRepoURL
		}
		if gitBranchName != "" {
			dagDeploymentConfig["branchName"] = gitBranchName
		}
		if gitDAGDir != "" {
			dagDeploymentConfig["dagDirectoryLocation"] = gitDAGDir
		}
		dagDeploymentConfig["syncInterval"] = gitSyncInterval
	}
	vars["dagDeployment"] = dagDeploymentConfig
	return nil
}

func readSSHKeyFile(sshFilePath string) (string, error) {
	fd, err := os.Open(sshFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errInvalidSSHKeyPath
		}
		return "", err
	}
	defer fd.Close()

	data, err := io.ReadAll(fd)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readKnownHostsFile(filePath, repoHost string) (string, error) {
	fd, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errInvalidKnownHostsPath
		}
		return "", err
	}
	defer fd.Close()

	scanner := bufio.NewScanner(fd)

	for scanner.Scan() {
		hostVal := scanner.Text()
		if strings.Contains(hostVal, repoHost) {
			return hostVal, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading known hosts file: %w", err)
	}

	return "", errHostNotPresent
}

func getURLHost(gitURL string) (string, error) {
	u, err := giturls.Parse(gitURL)
	if err != nil {
		return "", err
	}
	// Hostname will remove the port from the host if present, check if that is needed
	return u.Hostname(), nil
}

func GetDeploymentsErr(err error) error {
	return fmt.Errorf(houston.HoustonConnectionErrMsg, err) //nolint:all
}

var GetDeployments = func(ws string, client houston.ClientInterface) ([]houston.Deployment, error) {
	deployments, err := houston.Call(client.ListDeployments)(houston.ListDeploymentsRequest{WorkspaceID: ws})
	if err != nil {
		return deployments, GetDeploymentsErr(err)
	}

	return deployments, nil
}

var SelectDeployment = func(deployments []houston.Deployment, message string) (houston.Deployment, error) {
	// select deployment
	if len(deployments) == 0 {
		return houston.Deployment{}, nil
	}

	if len(deployments) == 1 {
		fmt.Println("Only one Deployment was found. Using the following Deployment by default: \n" +
			fmt.Sprintf("\n Deployment Name: %s", ansi.Bold(deployments[0].Label)) +
			fmt.Sprintf("\n Deployment ID: %s\n", ansi.Bold(deployments[0].ID)))

		return deployments[0], nil
	}

	tab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "DEPLOYMENT NAME", "RELEASE NAME", "DEPLOYMENT ID"},
	}

	fmt.Println(message)

	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].CreatedAt.Before(deployments[j].CreatedAt)
	})

	deployMap := map[string]houston.Deployment{}
	for i := range deployments {
		index := i + 1
		tab.AddRow([]string{strconv.Itoa(index), deployments[i].Label, deployments[i].ReleaseName, deployments[i].ID}, false)

		deployMap[strconv.Itoa(index)] = deployments[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := deployMap[choice]
	if !ok {
		return houston.Deployment{}, ErrInvalidDeploymentKey
	}
	return selected, nil
}
