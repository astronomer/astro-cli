package deployment

import "github.com/astronomer/astro-cli/internal/platform/apc/houston"

type CreateDeploymentRequest struct {
	Label             string
	WS                string
	ReleaseName       string
	CloudRole         string
	Executor          string
	AirflowVersion    string
	RuntimeVersion    string
	DAGDeploymentType string
	NFSLocation       string
	GitRepoURL        string
	GitRevision       string
	GitBranchName     string
	GitDAGDir         string
	SSHKey            string
	KnownHosts        string
	GitSyncInterval   int
	TriggererReplicas int
	ClusterID         string
	Mode              string
	// Namespace answers the namespace the platform asks for, when it asks
	// (--namespace): picked from the ones it offers, or a name of your own.
	Namespace string
}

// The images a Deployment can run, as ImageVersion.Image names them.
const (
	// ImageCertified is an Astronomer Certified image: Version is its
	// Airflow version.
	ImageCertified = "astronomer_certified"
	// ImageRuntime is an Astronomer Runtime image: Version is its Runtime
	// version.
	ImageRuntime = "runtime"
)

// ImageVersion is the image a Deployment runs, or is asked to.
type ImageVersion struct {
	Image   string
	Version string
}

// Label is the image as the CLI's tables and pickers name it:
// "Astronomer-Certified-2.0.0", "Runtime-4.2.0".
func (v ImageVersion) Label() string {
	if v.Image == ImageRuntime {
		return runtimeImageType + "-" + v.Version
	}
	return certifiedImageType + "-" + v.Version
}

// What a VersionChange did.
const (
	// VersionChangeStarted is an upgrade or migration that has begun. It
	// completes when an image of the desired version is deployed.
	VersionChangeStarted = "started"
	// VersionChangeCanceled is an upgrade or migration that was canceled.
	VersionChangeCanceled = "canceled"
	// VersionChangeNothingToCancel is a cancel with nothing under way.
	VersionChangeNothingToCancel = "nothing_to_cancel"
)

// VersionChange is what `astro deployment airflow upgrade`, `runtime
// upgrade` and `runtime migrate` did, or what canceling one did.
type VersionChange struct {
	// Deployment is the Deployment as Houston returned it: after the change
	// for a start, as it was read for a cancel.
	Deployment houston.Deployment
	// Action is one of the VersionChange constants.
	Action string
	// Current is the image the Deployment runs now.
	Current ImageVersion
	// Desired is the image a started change moves it to; nil otherwise.
	Desired *ImageVersion
}
