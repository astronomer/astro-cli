package deployment

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	giturls "github.com/whilp/git-urls"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	ErrKubernetesNamespaceNotAvailable = errors.New("no kubernetes namespaces are available")
	ErrNumberOutOfRange                = errors.New("number is out of available range")
	ErrMajorAirflowVersionUpgrade      = fmt.Errorf("Airflow 2.0 has breaking changes. To upgrade to Airflow 2.0, upgrade to %s first and make sure your Dags and configs are 2.0 compatible", minAirflowVersion)
	ErrKubernetesNamespaceNotSpecified = errors.New("no kubernetes namespaces specified")
	errInvalidSSHKeyPath               = errors.New("wrong path specified, no file exists for ssh key")
	errInvalidKnownHostsPath           = errors.New("wrong path specified, no file exists for known hosts")
	errHostNotPresent                  = errors.New("git repository host not present in known hosts file")

	ErrInvalidDeploymentKey = errors.New("invalid Deployment selected")

	errInvalidAirflowVersionSelection = errors.New("invalid Airflow version selection")
	errInvalidRuntimeVersionSelection = errors.New("invalid Runtime version selection")

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
	// currentVersion is the Runtime version the Deployment runs, as Houston
	// gives it: 3.0-1, not the 3000.0.1 it compares as.
	currentVersion string
}

func (e ErrInvalidRuntimeVersion) Error() string {
	return fmt.Sprintf("Error: You tried to set --desired-runtime-version to %s, but this Runtime Deployment "+
		"is already running %s. Please indicate a higher version of Runtime and try again.", e.desiredVersion, e.currentVersion)
}

func addTriggererReplicasArg(vars map[string]interface{}, appConfig *houston.AppConfig, triggererReplicas int) {
	if appConfig.Flags.TriggererEnabled && triggererReplicas != -1 {
		vars["triggererReplicas"] = triggererReplicas
	}
}

// Create creates an Airflow Deployment and returns it as Houston reported it.
// out is where the namespace picker draws its choices, when the platform asks
// for a namespace.
func Create(req *CreateDeploymentRequest, client houston.ClientInterface, out io.Writer, appConfig *houston.AppConfig) (*houston.Deployment, error) {
	vars := map[string]interface{}{"label": req.Label, "workspaceId": req.WS, "executor": req.Executor, "cloudRole": req.CloudRole}

	if req.ClusterID != "" {
		vars["clusterId"] = req.ClusterID
	}

	if req.Mode != "" {
		vars["mode"] = req.Mode
	}

	// Free-form entry wins when both are on, as it does in Houston, which
	// then skips the pre-created list. Asking for both used to have the
	// free-form answer replace the picked one.
	switch {
	case appConfig.Flags.NamespaceFreeFormEntry:
		namespace, err := getDeploymentNamespaceName(req.Namespace)
		if err != nil {
			return nil, err
		}
		vars["namespace"] = namespace
	case appConfig.Flags.ManualNamespaceNames:
		namespace, err := getDeploymentSelectionNamespaces(client, out, req.ClusterID, req.Namespace)
		if err != nil {
			return nil, err
		}
		vars["namespace"] = namespace
	case req.Namespace != "":
		return nil, errNamespaceNotAsked
	}

	if req.ReleaseName != "" && appConfig.ManualReleaseNames {
		vars["releaseName"] = req.ReleaseName
	}

	if req.AirflowVersion != "" {
		vars["airflowVersion"] = req.AirflowVersion
	} else if req.RuntimeVersion != "" {
		vars["runtimeVersion"] = req.RuntimeVersion
	}

	err := addDagDeploymentArgs(vars, req.DAGDeploymentType, req.NFSLocation, req.SSHKey, req.KnownHosts, req.GitRepoURL, req.GitRevision, req.GitBranchName, req.GitDAGDir, req.GitSyncInterval)
	if err != nil {
		return nil, err
	}

	addTriggererReplicasArg(vars, appConfig, req.TriggererReplicas)

	return houston.Call(client.CreateDeployment)(vars)
}

// Delete deletes a Deployment, and returns it as Houston reported it.
func Delete(id string, hardDelete bool, client houston.ClientInterface) (*houston.Deployment, error) {
	return houston.Call(client.DeleteDeployment)(houston.DeleteDeploymentRequest{DeploymentID: id, HardDelete: hardDelete})
}

// Adopt an existing operator-managed Airflow custom resource into Houston,
// and return the Deployment it now has.
func Adopt(req *houston.AdoptDeploymentRequest, client houston.ClientInterface) (*houston.Deployment, error) {
	return houston.Call(client.AdoptDeployment)(req)
}

// Unadopt releases an adopted deployment back to operator-only management, without touching
// the underlying Airflow custom resource, namespace, or metadata database. It
// returns the Deployment record it removed.
func Unadopt(id string, client houston.ClientInterface) (*houston.Deployment, error) {
	return houston.Call(client.UnadoptDeployment)(houston.UnadoptDeploymentRequest{DeploymentID: id})
}

// errNamespaceNotAvailable is a --namespace the platform does not offer.
type errNamespaceNotAvailable struct {
	given     string
	available []string
}

func (e errNamespaceNotAvailable) Error() string {
	return fmt.Sprintf("namespace %q is not one this platform offers; use one of: %s", e.given, strings.Join(e.available, ", "))
}

// getDeploymentSelectionNamespaces is the namespace for a new Deployment, from
// the ones the platform offers: given (--namespace) when it is one of them,
// or the one picked from a list drawn on out.
func getDeploymentSelectionNamespaces(client houston.ClientInterface, out io.Writer, clusterID, given string) (string, error) {
	logger.Debug("checking namespaces available for platform")

	names, err := houston.Call(client.GetAvailableNamespaces)(map[string]interface{}{"clusterID": clusterID})
	if err != nil {
		return "", err
	}

	if len(names) == 0 {
		return "", ErrKubernetesNamespaceNotAvailable
	}

	if given != "" {
		available := make([]string, 0, len(names))
		for _, namespace := range names {
			if namespace.Name == given {
				return given, nil
			}
			available = append(available, namespace.Name)
		}
		return "", errNamespaceNotAvailable{given: given, available: available}
	}

	list := picker.List{
		Header: []string{"AVAILABLE KUBERNETES NAMESPACES"},
		Ask:    []input.Option{input.About("a Kubernetes namespace"), input.AnsweredBy("--namespace")},
		InvalidAnswer: func(in string) error {
			if n, err := strconv.Atoi(in); err != nil || strconv.Itoa(n) != in {
				return ErrParsingInt{in: in}
			}
			return ErrNumberOutOfRange
		},
	}
	for _, namespace := range names {
		list.AddRow(false, namespace.Name)
	}
	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return "", err
	}
	return names[i].Name, nil
}

// getDeploymentNamespaceName is a namespace name of the person's own: given
// (--namespace), or asked for. It is checked as Houston will check it, so a
// bad name fails here with the rule rather than there with "Namespace name not
// formatted correctly.": not empty, at most 63 characters, a DNS-1123 label.
// Whether it is free, and the platform's pre-deployment webhook, only Houston
// can check.
func getDeploymentNamespaceName(given string) (string, error) {
	namespaceName := given
	if namespaceName == "" {
		var err error
		namespaceName, err = input.Text("\nKubernetes Namespace Name: ", input.AnsweredBy("--namespace"))
		if err != nil {
			return "", err
		}
	}
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" {
		return "", ErrKubernetesNamespaceNotSpecified
	}
	if len(namespaceName) > maxNamespaceLength || !namespaceNamePattern.MatchString(namespaceName) {
		return "", errInvalidNamespaceName{name: namespaceName}
	}
	return namespaceName, nil
}

// maxNamespaceLength and namespaceNamePattern are Houston's namespace rule.
const maxNamespaceLength = 63

var namespaceNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// errInvalidNamespaceName is a namespace name Houston would refuse.
type errInvalidNamespaceName struct{ name string }

func (e errInvalidNamespaceName) Error() string {
	return fmt.Sprintf("namespace %q is not a valid name: use at most %d lower-case letters, digits and '-', starting and ending with a letter or digit", e.name, maxNamespaceLength)
}

// errNamespaceNotAsked is --namespace on a platform that names namespaces
// itself: Houston ignores the argument then, so the name given would not
// be the one used.
var errNamespaceNotAsked = errors.New("--namespace is not used here: this platform names each Deployment's namespace itself")

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

// List returns the Deployments of the Workspace ws, or of every Workspace
// when all is set (on clusterID, if one is given), ordered by label, last
// first, as the CLI has always listed them.
func List(ws string, all bool, client houston.ClientInterface, clusterID string) ([]houston.Deployment, error) {
	deployments, err := getDeploymentsFromHouston(ws, all, client, clusterID)
	if err != nil {
		return nil, err
	}
	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Label > deployments[j].Label })
	return deployments, nil
}

// Update an airflow deployment, and return it as Houston reported it.
func Update(id, cloudRole string, args map[string]string, dagDeploymentType, nfsLocation, gitRepoURL, gitRevision, gitBranchName, gitDAGDir, sshKey, knownHosts, executor string, gitSyncInterval, triggererReplicas int, client houston.ClientInterface, appConfig *houston.AppConfig) (*houston.Deployment, error) {
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
		return nil, err
	}

	if appConfig.Flags.TriggererEnabled && triggererReplicas != -1 {
		vars["triggererReplicas"] = triggererReplicas
	}

	return houston.Call(client.UpdateDeployment)(vars)
}

// AirflowUpgrade starts upgrading a Deployment's Airflow. With no desired
// version it asks for one, drawing the choices on out.
func AirflowUpgrade(id, desiredAirflowVersion string, client houston.ClientInterface, out io.Writer) (*VersionChange, error) {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return nil, err
	}

	if deployment.RuntimeVersion != "" {
		return nil, errDeploymentNotOnAirflow
	}

	if desiredAirflowVersion == "" {
		selectedVersion, err := getAirflowVersionSelection(deployment.AirflowVersion, client, out)
		if err != nil {
			return nil, err
		}
		desiredAirflowVersion = selectedVersion
	}
	err = meetsAirflowUpgradeReqs(deployment.AirflowVersion, desiredAirflowVersion)
	if err != nil {
		return nil, err
	}

	vars := map[string]interface{}{"deploymentId": id, "desiredAirflowVersion": desiredAirflowVersion}

	d, err := houston.Call(client.UpdateDeploymentAirflow)(vars)
	if err != nil {
		return nil, err
	}

	return &VersionChange{
		Deployment: *d,
		Action:     VersionChangeStarted,
		Current:    ImageVersion{Image: ImageCertified, Version: d.AirflowVersion},
		Desired:    &ImageVersion{Image: ImageCertified, Version: d.DesiredAirflowVersion},
	}, nil
}

// currentImage is the image a Deployment runs now: Runtime when it has a
// Runtime version, Astronomer Certified otherwise. A cancel reports this, so
// it names what is running whichever image that is, not the one the command
// is about.
func currentImage(d *houston.Deployment) ImageVersion {
	if d.RuntimeVersion != "" {
		return ImageVersion{Image: ImageRuntime, Version: d.RuntimeVersion}
	}
	return ImageVersion{Image: ImageCertified, Version: d.AirflowVersion}
}

// AirflowUpgradeCancel cancels an Airflow upgrade that has not finished.
func AirflowUpgradeCancel(id string, client houston.ClientInterface) (*VersionChange, error) {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return nil, err
	}

	change := &VersionChange{
		Deployment: *deployment,
		Action:     VersionChangeNothingToCancel,
		Current:    currentImage(deployment),
	}
	if deployment.DesiredAirflowVersion != deployment.AirflowVersion {
		vars := map[string]interface{}{"deploymentId": id, "desiredAirflowVersion": deployment.AirflowVersion}

		_, err := houston.Call(client.UpdateDeploymentAirflow)(vars)
		if err != nil {
			return nil, err
		}
		change.Action = VersionChangeCanceled
	}
	return change, nil
}

// RuntimeUpgrade starts upgrading a Deployment to a newer Runtime version.
// With no desired version it asks for one, drawing the choices on out.
func RuntimeUpgrade(id, desiredRuntimeVersion string, client houston.ClientInterface, out io.Writer) (*VersionChange, error) {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return nil, err
	}

	if deployment.RuntimeVersion == "" && deployment.AirflowVersion != "" {
		return nil, errDeploymentNotOnRuntime
	}

	if desiredRuntimeVersion == "" {
		selectedVersion, err := getRuntimeVersionSelection(deployment.RuntimeVersion, deployment.RuntimeAirflowVersion, deployment.ClusterID, client, out)
		if err != nil {
			return nil, err
		}
		desiredRuntimeVersion = selectedVersion
	}
	err = meetsRuntimeUpgradeReqs(deployment.RuntimeVersion, desiredRuntimeVersion)
	if err != nil {
		return nil, err
	}

	vars := map[string]interface{}{"deploymentUuid": id, "desiredRuntimeVersion": desiredRuntimeVersion}

	d, err := houston.Call(client.UpdateDeploymentRuntime)(vars)
	if err != nil {
		return nil, err
	} else if d == nil {
		return nil, errRuntimeUpdateFailed
	}

	return &VersionChange{
		Deployment: *d,
		Action:     VersionChangeStarted,
		Current:    ImageVersion{Image: ImageRuntime, Version: d.RuntimeVersion},
		Desired:    &ImageVersion{Image: ImageRuntime, Version: desiredRuntimeVersion},
	}, nil
}

// RuntimeUpgradeCancel cancels a Runtime upgrade that has not finished.
func RuntimeUpgradeCancel(id string, client houston.ClientInterface) (*VersionChange, error) {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return nil, err
	}

	change := &VersionChange{
		Deployment: *deployment,
		Action:     VersionChangeNothingToCancel,
		Current:    currentImage(deployment),
	}
	if deployment.DesiredRuntimeVersion != deployment.RuntimeVersion {
		vars := map[string]interface{}{"deploymentUuid": id}

		_, err := houston.Call(client.CancelUpdateDeploymentRuntime)(vars)
		var notServed houston.ErrAPINotImplemented
		switch {
		case errors.As(err, &notServed):
			// Houston 1.0.43 removed cancelRuntimeUpdate along with the
			// pending desired version: a Runtime upgrade there is a direct
			// upsert, with nothing left to cancel.
			// Its GetDeployment has no desired version for the check above
			// to compare, so this is where that is known.
			return change, nil
		case err != nil:
			return nil, err
		}
		change.Action = VersionChangeCanceled
	}
	return change, nil
}

// RuntimeMigrate starts migrating a Deployment from an Astronomer Certified
// image to the newest Runtime release for its Airflow version.
func RuntimeMigrate(deploymentID string, client houston.ClientInterface) (*VersionChange, error) {
	deployment, err := houston.Call(client.GetDeployment)(deploymentID)
	if err != nil {
		return nil, err
	}

	if deployment.AirflowVersion == "" || deployment.RuntimeVersion != "" {
		return nil, errDeploymentAlreadyOnRuntime
	}

	vars := make(map[string]interface{})
	vars["airflowVersion"] = deployment.AirflowVersion
	vars["clusterId"] = deployment.ClusterID
	runtimeReleases, err := houston.Call(client.GetRuntimeReleases)(vars)
	if err != nil {
		return nil, err
	}

	var latestRuntimeRelease *semver.Version
	for idx := range runtimeReleases {
		runtimeVersion, _ := semver.NewVersion(runtimeReleases[idx].Version) //nolint:errcheck // error deliberately ignored in this shell code
		if latestRuntimeRelease == nil {
			latestRuntimeRelease = runtimeVersion
		} else if runtimeVersion != nil && !latestRuntimeRelease.GreaterThan(runtimeVersion) {
			latestRuntimeRelease = runtimeVersion
		}
	}

	if latestRuntimeRelease == nil {
		return nil, errInvalidAirflowVersion
	}
	desiredRuntimeVersion := latestRuntimeRelease.String()

	vars = map[string]interface{}{"deploymentUuid": deploymentID, "desiredRuntimeVersion": desiredRuntimeVersion}
	resp, err := houston.Call(client.UpdateDeploymentRuntime)(vars)
	if err != nil {
		return nil, err
	} else if resp == nil {
		return nil, errRuntimeUpdateFailed
	}

	return &VersionChange{
		Deployment: *resp,
		Action:     VersionChangeStarted,
		Current:    ImageVersion{Image: ImageCertified, Version: deployment.AirflowVersion},
		Desired:    &ImageVersion{Image: ImageRuntime, Version: desiredRuntimeVersion},
	}, nil
}

// RuntimeMigrateCancel cancels a migration to Runtime that has not finished.
func RuntimeMigrateCancel(id string, client houston.ClientInterface) (*VersionChange, error) {
	deployment, err := houston.Call(client.GetDeployment)(id)
	if err != nil {
		return nil, err
	}

	if deployment.RuntimeVersion == "" && deployment.DesiredRuntimeVersion != "" && deployment.AirflowVersion != "" {
		vars := map[string]interface{}{"deploymentUuid": id}
		_, err := houston.Call(client.CancelUpdateDeploymentRuntime)(vars)
		if err != nil {
			return nil, err
		}
		return &VersionChange{
			Deployment: *deployment,
			Action:     VersionChangeCanceled,
			Current:    currentImage(deployment),
		}, nil
	}

	return &VersionChange{
		Deployment: *deployment,
		Action:     VersionChangeNothingToCancel,
		Current:    currentImage(deployment),
	}, nil
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

	list := picker.List{
		Header:  []string{"AIRFLOW VERSION"},
		Ask:     []input.Option{input.About("an Airflow version"), input.AnsweredBy("--desired-airflow-version")},
		Invalid: errInvalidAirflowVersionSelection,
	}

	var filteredVersions []string

	for _, v := range airflowVersions {
		vv, _ := semver.NewVersion(v) //nolint:errcheck // error deliberately ignored in this shell code
		if currentAirflowVersion.LessThan(vv) {
			filteredVersions = append(filteredVersions, v)
			list.AddRow(false, fmt.Sprintf("%s-%s", certifiedImageType, v))
		}
	}

	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return "", err
	}
	return filteredVersions[i], nil
}

func getRuntimeVersionSelection(runtimeVersion, airflowVersion, clusterID string, client houston.ClientInterface, out io.Writer) (string, error) {
	currentRuntimeVersion, err := semver.NewVersion(normalizeRuntimeVersion(runtimeVersion))
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

	list := picker.List{
		Header:  []string{"RUNTIME VERSION"},
		Ask:     []input.Option{input.About("a Runtime version"), input.AnsweredBy("--desired-runtime-version")},
		Invalid: errInvalidRuntimeVersionSelection,
	}

	var filteredVersions []string

	for _, v := range runtimeVersions {
		runtimeVersion, err := semver.NewVersion(normalizeRuntimeVersion(v.Version))
		if err != nil {
			continue
		}
		airflowVersion, err := semver.NewVersion(v.AirflowVersion)
		if err != nil {
			continue
		}
		if currentRuntimeVersion.LessThan(runtimeVersion) && !currentAirflowVersion.GreaterThan(airflowVersion) {
			filteredVersions = append(filteredVersions, v.Version)
			list.AddRow(false, fmt.Sprintf("%s-%s", runtimeImageType, v.Version))
		}
	}

	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return "", err
	}
	return filteredVersions[i], nil
}

func meetsAirflowUpgradeReqs(airflowVersion, desiredAirflowVersion string) error {
	upgradeVersion := "2" // an upgrade to Airflow 2 is the one with requirements
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

// airflowV3RuntimePattern is a Runtime version for Airflow 3, "3.0-1": not
// semver, and read by semver as 3.0.0-1, a prerelease below every 3.x.
// airflowV3MajorScale is the factor Houston scales an Airflow 3 Runtime's major by.
const airflowV3MajorScale = 1000

var airflowV3RuntimePattern = regexp.MustCompile(`^(\d+)\.(\d+)-(\d+)(?:-[a-zA-Z0-9.-]+)?$`)

// normalizeRuntimeVersion is a Runtime version as Houston compares it: an
// Airflow 3 version M.m-p becomes (M*1000).m.p, so it orders above every
// Airflow 2 Runtime (12.x, 13.x), as Houston orders it. Anything else is
// unchanged.
func normalizeRuntimeVersion(v string) string {
	m := airflowV3RuntimePattern.FindStringSubmatch(v)
	if m == nil {
		return v
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return v
	}
	return fmt.Sprintf("%d.%s.%s", major*airflowV3MajorScale, m[2], m[3])
}

func meetsRuntimeUpgradeReqs(runtimeVersion, desiredRuntimeVersion string) error {
	desiredVersion, err := semver.NewVersion(normalizeRuntimeVersion(desiredRuntimeVersion))
	if err != nil {
		return err
	}

	currentVersion, err := semver.NewVersion(normalizeRuntimeVersion(runtimeVersion))
	if err != nil {
		return err
	}

	if currentVersion.Compare(desiredVersion) == 0 {
		return ErrInvalidRuntimeVersion{desiredVersion: desiredRuntimeVersion, currentVersion: runtimeVersion}
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
	return fmt.Errorf(houston.HoustonConnectionErrMsg, err)
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

	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].CreatedAt.Before(deployments[j].CreatedAt)
	})

	list := picker.List{
		Title:   message,
		Header:  []string{"DEPLOYMENT NAME", "RELEASE NAME", "DEPLOYMENT ID"},
		Ask:     []input.Option{input.About("a deployment")},
		Invalid: ErrInvalidDeploymentKey,
	}
	for i := range deployments {
		list.AddRow(false, deployments[i].Label, deployments[i].ReleaseName, deployments[i].ID)
	}
	i, err := list.Pick(os.Stderr, os.Stdin)
	if err != nil {
		return houston.Deployment{}, err
	}
	return deployments[i], nil
}
