package deployment

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/settings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/fatih/camelcase"
	"github.com/sirupsen/logrus"
	giturls "github.com/whilp/git-urls"
)

var (
	minAirflowVersion = "1.10.14"

	ErrKubernetesNamespaceNotAvailable = errors.New("no kubernetes namespaces are available")
	ErrNumberOutOfRange                = errors.New("number is out of available range")
	ErrMajorAirflowVersionUpgrade      = fmt.Errorf("Airflow 2.0 has breaking changes. To upgrade to Airflow 2.0, upgrade to %s first and make sure your DAGs and configs are 2.0 compatible", minAirflowVersion) //nolint:golint,stylecheck
	ErrKubernetesNamespaceNotSpecified = errors.New("no kubernetes namespaces specified")
	errInvalidSSHKeyPath               = errors.New("wrong path specified, no file exists for ssh key")
	errInvalidKnownHostsPath           = errors.New("wrong path specified, no file exists for known hosts")
	errHostNotPresent                  = errors.New("git repository host not present in known hosts file")
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

func checkManualReleaseNames(client houston.ClientInterface) bool {
	logrus.Debug("Checking checkManualReleaseNames through appConfig from houston-api")

	config, err := client.GetAppConfig()
	if err != nil {
		return false
	}

	return config.ManualReleaseNames
}

// CheckNFSMountDagDeployment returns true when we can set custom NFS location for dags
func CheckNFSMountDagDeployment(client houston.ClientInterface) bool {
	logrus.Debug("Checking checkNFSMountDagDeployment through appConfig from houston-api")

	config, err := client.GetAppConfig()
	if err != nil {
		return false
	}

	return config.Flags.NfsMountDagDeployment
}

func CheckHardDeleteDeployment(client houston.ClientInterface) bool {
	logrus.Debug("Checking for hard delete deployment flag")
	config, err := client.GetAppConfig()
	if err != nil {
		return false
	}
	return config.Flags.HardDeleteDeployment
}

func CheckPreCreateNamespaceDeployment(client houston.ClientInterface) bool {
	logrus.Debug("Checking for pre created deployment flag")
	config, err := client.GetAppConfig()
	if err != nil {
		return false
	}
	return config.Flags.ManualNamespaceNames
}

func CheckNamespaceFreeFormEntryDeployment(client houston.ClientInterface) bool {
	config, err := client.GetAppConfig()
	if err != nil {
		return false
	}
	return config.Flags.NamespaceFreeFormEntry
}

func CheckTriggererEnabled(client houston.ClientInterface) bool {
	logrus.Debug("Checking for triggerer flag")
	config, err := client.GetAppConfig()
	if err != nil {
		return false
	}
	return config.Flags.TriggererEnabled
}

// Create airflow deployment
func Create(label, ws, releaseName, cloudRole, executor, airflowVersion, dagDeploymentType, nfsLocation, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, sshKey, knownHosts string, gitSyncInterval, triggererReplicas int, client houston.ClientInterface, out io.Writer) error {
	vars := map[string]interface{}{"label": label, "workspaceId": ws, "executor": executor, "cloudRole": cloudRole}

	if CheckPreCreateNamespaceDeployment(client) {
		namespace, err := getDeploymentSelectionNamespaces(client, out)
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
	d, err := client.CreateDeployment(vars)
	if err != nil {
		return err
	}

	tab := newTableOut()
	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.ID, "-", d.AirflowVersion}, false)

	splitted := []string{"Celery", ""}

	if executor != "" {
		// trim executor from console message
		splitted = camelcase.Split(executor)
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
	if executor == houston.CeleryExecutorType || executor == "" {
		tab.SuccessMsg += fmt.Sprintf("\n Flower Dashboard: %s", flowerURL)
	}
	tab.Print(out)

	return nil
}

func Delete(id string, hardDelete bool, client houston.ClientInterface, out io.Writer) error {
	_, err := client.DeleteDeployment(id, hardDelete)
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
func getDeploymentSelectionNamespaces(client houston.ClientInterface, out io.Writer) (string, error) {
	tab := &printutil.Table{
		Padding:        []int{30},
		DynamicPadding: true,
		Header:         []string{"AVAILABLE KUBERNETES NAMESPACES"},
	}

	logrus.Debug("checking namespaces available for platform")
	tab.GetUserInput = true

	names, err := client.GetAvailableNamespaces()
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
	i, err := strconv.ParseInt(in, 10, 64)
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

// List all airflow deployments
func List(ws string, all bool, client houston.ClientInterface, out io.Writer) error {
	listDeploymentRequest := houston.ListDeploymentsRequest{}
	if !all {
		listDeploymentRequest.WorkspaceID = ws
	}

	deployments, err := client.ListDeployments(listDeploymentRequest)
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
		tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, currentTag, d.AirflowVersion}, false)
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

	if CheckTriggererEnabled(client) {
		vars["triggererReplicas"] = triggererReplicas
	}

	d, err := client.UpdateDeployment(vars)
	if err != nil {
		return err
	}

	tab := newTableOut()
	currentTag := d.DeploymentInfo.Current
	if currentTag == "" {
		currentTag = "?"
	}
	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.ID, currentTag, d.AirflowVersion}, false)
	tab.SuccessMsg = "\n Successfully updated deployment"
	tab.Print(out)

	return nil
}

// Upgrade airflow deployment
func AirflowUpgrade(id, desiredAirflowVersion string, client houston.ClientInterface, out io.Writer) error {
	deployment, err := client.GetDeployment(id)
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

	d, err := client.UpdateDeploymentAirflow(vars)
	if err != nil {
		return err
	}

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
func AirflowUpgradeCancel(id string, client houston.ClientInterface, out io.Writer) error {
	deployment, err := client.GetDeployment(id)
	if err != nil {
		return err
	}

	if deployment.DesiredAirflowVersion != deployment.AirflowVersion {
		vars := map[string]interface{}{"deploymentId": id, "desiredAirflowVersion": deployment.AirflowVersion}

		_, err := client.UpdateDeploymentAirflow(vars)
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

func getAirflowVersionSelection(airflowVersion string, client houston.ClientInterface, out io.Writer) (string, error) {
	currentAirflowVersion, err := semver.NewVersion(airflowVersion)
	if err != nil {
		return "", err
	}
	// prepare list of AC airflow versions
	config, err := client.GetDeploymentConfig()
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
