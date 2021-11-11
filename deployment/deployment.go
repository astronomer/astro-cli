package deployment

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	giturls "github.com/whilp/git-urls"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/settings"
	"github.com/fatih/camelcase"
)

var (
	minAirflowVersion = "1.10.14"

	celeryExecutor = "CeleryExecutor"

	volumeDeploymentType  = "volume"
	gitSyncDeploymentType = "git_sync"

	ErrKubernetesNamespaceNotAvailable = errors.New("no kubernetes namespaces are available")
	ErrNumberOutOfRange                = errors.New("number is out of available range")
	ErrMajorAirflowVersionUpgrade      = fmt.Errorf("Airflow 2.0 has breaking changes. To upgrade to Airflow 2.0, upgrade to %s first and make sure your DAGs and configs are 2.0 compatible", minAirflowVersion) //nolint:golint,stylecheck
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

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "TAG", "AIRFLOW VERSION"},
	}
}

// AppVersion returns application version from houston-api
func AppVersion(client *houston.Client) (*houston.AppConfig, error) {
	req := houston.Request{
		Query: houston.AppVersionRequest,
	}
	r, err := req.DoWithClient(client)
	if err != nil {
		return nil, err
	}

	return r.Data.GetAppConfig, nil
}

// AppConfig returns application config from houston-api
func AppConfig(client *houston.Client) (*houston.AppConfig, error) {
	logrus.Debug("Checking AppConfig from houston-api")
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
	logrus.Debug("Checking checkManualReleaseNames through appConfig from houston-api")
	req := houston.Request{
		Query: houston.AppConfigRequest,
	}
	r, err := req.DoWithClient(client)
	if err != nil {
		return false
	}

	return r.Data.GetAppConfig.ManualReleaseNames
}

// CheckNFSMountDagDeployment returns true when we can set custom NFS location for dags
func CheckNFSMountDagDeployment(client *houston.Client) bool {
	logrus.Debug("Checking checkNFSMountDagDeployment through appConfig from houston-api")
	req := houston.Request{
		Query: houston.AppConfigRequest,
	}
	r, err := req.DoWithClient(client)
	if err != nil {
		return false
	}

	return r.Data.GetAppConfig.Flags.NfsMountDagDeployment
}

func CheckHardDeleteDeployment(client *houston.Client) bool {
	logrus.Debug("Checking for hard delete deployment flag")
	appConfig, err := AppConfig(client)
	if err != nil {
		return false
	}
	return appConfig.Flags.HardDeleteDeployment
}

func CheckPreCreateNamespaceDeployment(client *houston.Client) bool {
	logrus.Debug("Checking for pre created deployment flag")
	appConfig, err := AppConfig(client)
	if err != nil {
		return false
	}
	return appConfig.Flags.ManualNamespaceNames
}

func CheckTriggererEnabled(client *houston.Client) bool {
	logrus.Debug("Checking for triggerer flag")
	appConfig, err := AppConfig(client)
	if err != nil {
		return false
	}
	return appConfig.Flags.TriggererEnabled
}

// Create airflow deployment
func Create(label, ws, releaseName, cloudRole, executor, airflowVersion, dagDeploymentType, nfsLocation, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, sshKey, knownHosts string, gitSyncInterval, triggererReplicas int, client *houston.Client, out io.Writer) error {
	vars := map[string]interface{}{"label": label, "workspaceId": ws, "executor": executor, "cloudRole": cloudRole}

	if CheckPreCreateNamespaceDeployment(client) {
		namespace, err := getDeploymentSelectionNamespaces(client, out)
		if err != nil {
			return err
		}
		vars["namespace"] = namespace
	}

	if releaseName != "" && checkManualReleaseNames(client) {
		vars["releaseName"] = releaseName
	}

	if airflowVersion != "" {
		vars["airflowVersion"] = airflowVersion
	}

	err := addDagDeploymentArgs(vars, dagDeploymentType, nfsLocation, sshKey, knownHosts, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, gitSyncInterval)
	if err != nil {
		return err
	}

	if CheckTriggererEnabled(client) {
		vars["triggererReplicas"] = triggererReplicas
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
	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.ID, "-", d.AirflowVersion}, false)

	splitted := []string{"Celery", ""}

	if executor != "" {
		// trim executor from console message
		splitted = camelcase.Split(executor)
	}

	var airflowURL, flowerURL string
	for _, url := range r.Data.CreateDeployment.Urls {
		if url.Type == "airflow" {
			airflowURL = url.URL
		}
		if url.Type == "flower" {
			flowerURL = url.URL
		}
	}

	tab.SuccessMsg =
		fmt.Sprintf("\n Successfully created deployment with %s executor", splitted[0]) +
			". Deployment can be accessed at the following URLs \n" +
			fmt.Sprintf("\n Airflow Dashboard: %s", airflowURL)

	// The Flower URL is specific to CeleryExecutor only
	if executor == celeryExecutor || executor == "" {
		tab.SuccessMsg += fmt.Sprintf("\n Flower Dashboard: %s", flowerURL)
	}
	tab.Print(out)

	return nil
}

func Delete(id string, hardDelete bool, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.DeploymentDeleteRequest,
		Variables: map[string]interface{}{"deploymentId": id, "deploymentHardDelete": hardDelete},
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

// list all available namespaces
func getDeploymentSelectionNamespaces(client *houston.Client, out io.Writer) (string, error) {
	tab := &printutil.Table{
		Padding:        []int{30},
		DynamicPadding: true,
		Header:         []string{"AVAILABLE KUBERNETES NAMESPACES"},
	}

	logrus.Debug("checking namespaces available for platform")
	tab.GetUserInput = true

	req := houston.Request{
		Query: houston.AvailableNamespacesGetRequest,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return "", err
	}

	names := r.Data.GetDeploymentNamespaces

	if len(names) == 0 {
		return "", ErrKubernetesNamespaceNotAvailable
	}

	for _, namespace := range names {
		name := namespace.Name

		tab.AddRow([]string{name}, false)
	}

	tab.Print(out)

	in := input.Text("\n> ")
	i, err := strconv.ParseInt(in, 10, 64)
	if err != nil {
		return "", ErrParsingInt{in: in}
	}
	if i > int64(len(names)) {
		return "", ErrNumberOutOfRange
	}
	return names[i-1].Name, nil
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
	for i := range deployments {
		d := deployments[i]

		currentTag := d.DeploymentInfo.Current
		if currentTag == "" {
			currentTag = "?"
		}
		tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, currentTag, d.AirflowVersion}, false)
	}

	return tab.Print(out)
}

// Update an airflow deployment
func Update(id, cloudRole string, args map[string]string, dagDeploymentType, nfsLocation, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, sshKey, knownHosts string, gitSyncInterval, triggererReplicas int, client *houston.Client, out io.Writer) error {
	vars := map[string]interface{}{"deploymentId": id, "payload": args, "cloudRole": cloudRole}

	// sync with commander only when we have cloudRole
	if cloudRole != "" {
		vars["sync"] = true
	}

	// adds dag deployment args to the vars map
	err := addDagDeploymentArgs(vars, dagDeploymentType, nfsLocation, sshKey, knownHosts, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, gitSyncInterval)
	if err != nil {
		return err
	}

	if CheckTriggererEnabled(client) {
		vars["triggererReplicas"] = triggererReplicas
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
	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.ID, d.AirflowVersion}, false)
	tab.SuccessMsg = "\n Successfully updated deployment"
	tab.Print(out)

	return nil
}

// Upgrade airflow deployment
func AirflowUpgrade(id, desiredAirflowVersion string, client *houston.Client, out io.Writer) error {
	deployment, err := getDeployment(id, client)
	if err != nil {
		return err
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
	tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, d.AirflowVersion}, false)

	tab.SuccessMsg = fmt.Sprintf("\nThe upgrade from Airflow %s to %s has been started. ", d.AirflowVersion, d.DesiredAirflowVersion) +
		fmt.Sprintf("To complete this process, add an Airflow %s image to your Dockerfile and deploy to Astronomer.\n", d.DesiredAirflowVersion) +
		"To cancel, run: \n $ astro deployment airflow upgrade --cancel\n"

	tab.Print(out)

	return nil
}

// Upgrade airflow deployment
func AirflowUpgradeCancel(id string, client *houston.Client, out io.Writer) error {
	deployment, err := getDeployment(id, client)
	if err != nil {
		return err
	}

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

		text := "\nAirflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running Airflow %s.\n"
		fmt.Fprintf(out, text, deployment.AirflowVersion)
		return nil
	}

	text := "\nNothing to cancel. You are currently running Airflow %s and you have not indicated that you want to upgrade."
	fmt.Fprintf(out, text, deployment.AirflowVersion)
	return nil
}

func getAirflowVersionSelection(airflowVersion string, client *houston.Client, out io.Writer) (string, error) {
	currentAirflowVersion, err := semver.NewVersion(airflowVersion)
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

	var filteredVersions []string

	for _, v := range airflowVersions {
		vv, _ := semver.NewVersion(v)
		// false means no colors
		if currentAirflowVersion.LessThan(vv) {
			filteredVersions = append(filteredVersions, v)
			t.AddRow([]string{v}, false)
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

func getDeployment(deploymentID string, client *houston.Client) (*houston.Deployment, error) {
	vars := map[string]interface{}{"id": deploymentID}

	req := houston.Request{
		Query:     houston.DeploymentGetRequest,
		Variables: vars,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return nil, err
	}

	return &r.Data.GetDeployment, nil
}

func meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion string) error {
	upgradeVersion := strconv.FormatUint(settings.AirflowVersionTwo, 10)
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

// addDagDeploymentArgs adds dag deployment argument to houston request map
func addDagDeploymentArgs(vars map[string]interface{}, dagDeploymentType, nfsLocation, sshKey, knownHosts, gitRepoURL, gitRevision, gitBranchName, gitDAGDir string, gitSyncInterval int) error {
	if dagDeploymentType == volumeDeploymentType && nfsLocation != "" {
		vars["dagDeployment"] = map[string]string{"nfsLocation": nfsLocation, "type": dagDeploymentType}
	}

	if dagDeploymentType == gitSyncDeploymentType {
		dagDeploymentConfig := map[string]interface{}{"type": dagDeploymentType}
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
		vars["dagDeployment"] = dagDeploymentConfig
	}
	return nil
}

func readSSHKeyFile(sshFilePath string) (string, error) {
	fd, err := os.Open(sshFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("wrong path specified, no file exists for ssh key")
		}
		return "", err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readKnownHostsFile(filePath, repoHost string) (string, error) {
	fd, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("wrong path specified, no file exists for known hosts")
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
		return "", errors.Wrapf(err, "error reading known hosts file")
	}

	return "", errors.New("git repository host not present in known hosts file")
}

func getURLHost(gitURL string) (string, error) {
	u, err := giturls.Parse(gitURL)
	if err != nil {
		return "", err
	}
	// Hostname will remove the port from the host if present, check if that is needed
	return u.Hostname(), nil
}
