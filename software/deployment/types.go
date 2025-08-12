package deployment

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
}
