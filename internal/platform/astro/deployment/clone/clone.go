// Package clone builds the one create request that copies an Astro
// Deployment, from the Deployment as the API's GET returns it.
//
// It is the core of `astro deployment create --clone`: a pure function of the
// source, with no reads, no prints and no config. The command reads the
// source, sends what Request builds, and prints the Notes on stderr.
//
// What a GET does not return cannot be copied: secret environment variable
// values, a custom workload identity (the GET gives only the identity in
// effect, which for a default one is the source's own), Remote Execution
// agents, low latency, API server and event scheduler autoscaling, alerts and
// notification channels, Deployment tokens, roles, the image and the DAGs.
package clone

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// Note is something about the source that the copy does not carry, for the
// command to tell the person cloning it.
type Note string

var (
	errNoName = errors.New("the new Deployment needs a name")
	errNoType = errors.New("the Deployment has no type, so it cannot be copied")
)

// Request is the create request that copies src as a Deployment named name,
// in workspaceID, or in src's own Workspace when workspaceID is empty. A
// non-nil description replaces src's. The Notes say what src has that the
// request leaves out.
func Request(src *astrov1.Deployment, name, workspaceID string, description *string) (astrov1.CreateDeploymentRequest, []Note, error) {
	var req astrov1.CreateDeploymentRequest
	if name == "" {
		return req, nil, errNoName
	}
	if src.Type == nil {
		return req, nil, errNoType
	}
	c := common(src, name, workspaceID, description)
	notes := c.notes

	var err error
	switch *src.Type {
	case astrov1.DeploymentTypeSTANDARD:
		var r astrov1.CreateStandardDeploymentRequest
		if r, err = standard(src, &c); err == nil {
			err = req.FromCreateStandardDeploymentRequest(r)
		}
	case astrov1.DeploymentTypeDEDICATED:
		var r astrov1.CreateDedicatedDeploymentRequest
		var reNotes []Note
		if r, reNotes, err = dedicated(src, &c); err == nil {
			notes = append(notes, reNotes...)
			err = req.FromCreateDedicatedDeploymentRequest(r)
		}
	case astrov1.DeploymentTypeHYBRID:
		var r astrov1.CreateHybridDeploymentRequest
		if r, err = hybrid(src, &c); err == nil {
			err = req.FromCreateHybridDeploymentRequest(r)
		}
	default:
		err = fmt.Errorf("a %s Deployment cannot be copied", *src.Type)
	}
	if err != nil {
		return astrov1.CreateDeploymentRequest{}, nil, err
	}
	return req, notes, nil
}

// fields are what every type's request takes, in the source's terms.
type fields struct {
	name, workspaceID, runtimeVersion string
	description                       *string
	executor                          *string
	isCicdEnforced, isDagDeployOn     bool
	contactEmails                     *[]string
	env                               *[]astrov1.DeploymentEnvironmentVariableRequest
	notes                             []Note
}

func common(src *astrov1.Deployment, name, workspaceID string, description *string) fields {
	f := fields{
		name:           name,
		workspaceID:    workspaceID,
		runtimeVersion: src.AstroRuntimeVersion,
		description:    src.Description,
		isCicdEnforced: src.IsCicdEnforced,
		isDagDeployOn:  src.IsDagDeployEnabled,
	}
	if f.workspaceID == "" {
		f.workspaceID = src.WorkspaceId
	}
	if f.runtimeVersion == "" {
		f.runtimeVersion = src.RuntimeVersion
	}
	if description != nil {
		f.description = description
	}
	if src.Executor != nil {
		e := string(*src.Executor)
		f.executor = &e
	}
	if src.ContactEmails != nil {
		emails := append([]string{}, *src.ContactEmails...)
		f.contactEmails = &emails
	}
	f.env, f.notes = environment(src)
	f.notes = append(f.notes, identityNotes(src)...)
	return f
}

// environment copies the source's plain variables. A secret one's value never
// comes back from a GET, and the create refuses a variable with none, so it
// is left out and named in a Note.
func environment(src *astrov1.Deployment) (*[]astrov1.DeploymentEnvironmentVariableRequest, []Note) {
	if src.EnvironmentVariables == nil || len(*src.EnvironmentVariables) == 0 {
		return nil, nil
	}
	vars := []astrov1.DeploymentEnvironmentVariableRequest{}
	var secrets []string
	for _, v := range *src.EnvironmentVariables {
		if v.IsSecret {
			secrets = append(secrets, v.Key)
			continue
		}
		value := ""
		if v.Value != nil {
			value = *v.Value
		}
		vars = append(vars, astrov1.DeploymentEnvironmentVariableRequest{Key: v.Key, Value: &value})
	}
	var notes []Note
	if len(secrets) > 0 {
		notes = append(notes, Note(fmt.Sprintf(
			"Not copied: secret environment variables %s. The API does not return their values; set them on the new Deployment with `astro deployment variable create --secret`.",
			strings.Join(secrets, ", "))))
	}
	if len(vars) == 0 {
		return nil, notes
	}
	return &vars, notes
}

// The workload identities Astro gives a Deployment of its own. The GET
// carries the Deployment's namespace, which is its release name, and its
// cluster id, but not the cloud account, so the account part matches any.
var (
	awsDeploymentRole = regexp.MustCompile(`^arn:aws:iam::[^:]+:role/astro-(.+)$`)
	awsClusterRole    = regexp.MustCompile(`^arn:aws:iam::[^:]+:role/AirflowS3Logs-(.+)$`)
	gcpServiceAccount = regexp.MustCompile(`^([^@]+)@[^@]+\.iam\.gserviceaccount\.com$`)
)

// gcpServiceAccountIDMax is GCP's cap on a service account id, to which the
// platform cuts "astro-" and the release name.
const gcpServiceAccountIDMax = 30

// isDefaultIdentity reports whether id is the identity Astro made for src,
// rather than one somebody set. Only a set one would be worth copying, and
// the GET cannot say which it is, so the copy takes the default either way.
//
// The default is the one the platform picks:
//   - GCP, any type: the service account "astro-<release>", cut to 30
//     characters;
//   - AWS STANDARD or DEDICATED: the per-Deployment role astro-<release>;
//   - AWS HYBRID: the cluster's role AirflowS3Logs-<cluster id>;
//   - anything else, Azure included: none.
//
// So a pattern is a default only for the cloud and type it belongs to, and
// only with the source's own release name or cluster: a dedicated Deployment
// set to its cluster's AirflowS3Logs role has a custom identity.
func isDefaultIdentity(src *astrov1.Deployment, id string) bool {
	if id == "" {
		return true
	}
	if src.CloudProvider == nil || src.Type == nil {
		return false
	}
	switch *src.CloudProvider {
	case astrov1.DeploymentCloudProviderGCP:
		m := gcpServiceAccount.FindStringSubmatch(id)
		if m == nil {
			return false
		}
		want := "astro-" + src.Namespace
		if len(want) > gcpServiceAccountIDMax {
			want = strings.Trim(want[:gcpServiceAccountIDMax], "-")
		}
		return m[1] == want
	case astrov1.DeploymentCloudProviderAWS:
		if *src.Type == astrov1.DeploymentTypeSTANDARD || *src.Type == astrov1.DeploymentTypeDEDICATED {
			m := awsDeploymentRole.FindStringSubmatch(id)
			return len(m) == 2 && m[1] == src.Namespace
		}
		m := awsClusterRole.FindStringSubmatch(id)
		return len(m) == 2 && src.ClusterId != nil && m[1] == *src.ClusterId
	case astrov1.DeploymentCloudProviderAZURE:
		// No default: any identity on an Azure Deployment was set.
		return false
	}
	return false
}

func identityNotes(src *astrov1.Deployment) []Note {
	var notes []Note
	if id := src.EffectiveWorkloadIdentity; id != nil && !isDefaultIdentity(src, *id) {
		notes = append(notes, Note(fmt.Sprintf(
			"Not copied: the workload identity %s, which looks custom. The new Deployment has Astro's default; set it with `astro deployment update --workload-identity` if it should match.",
			*id)))
	}
	// --workload-identity sets the primary identity only; nothing in the CLI
	// sets the disaster recovery one.
	if id := src.EffectiveDRWorkloadIdentity; id != nil && !isDefaultIdentity(src, *id) {
		notes = append(notes, Note(fmt.Sprintf(
			"Not copied: the disaster recovery workload identity %s, which looks custom. The new Deployment has Astro's default, and the CLI cannot set this one: set it in the Astro UI or with the API if it should match.",
			*id)))
	}
	return notes
}

func standard(src *astrov1.Deployment, c *fields) (astrov1.CreateStandardDeploymentRequest, error) {
	if src.CloudProvider == nil || src.Region == nil {
		return astrov1.CreateStandardDeploymentRequest{}, errors.New("the Deployment has no cloud provider or region, so it cannot be copied")
	}
	r := astrov1.CreateStandardDeploymentRequest{
		Name:                 c.name,
		WorkspaceId:          c.workspaceID,
		Type:                 new(astrov1.CreateStandardDeploymentRequestTypeSTANDARD),
		AstroRuntimeVersion:  &c.runtimeVersion,
		Description:          c.description,
		IsCicdEnforced:       &c.isCicdEnforced,
		IsDagDeployEnabled:   &c.isDagDeployOn,
		ContactEmails:        c.contactEmails,
		EnvironmentVariables: c.env,
		CloudProvider:        new(astrov1.CreateStandardDeploymentRequestCloudProvider(*src.CloudProvider)),
		Region:               new(*src.Region),
		IsHighAvailability:   src.IsHighAvailability,
		IsDevelopmentMode:    src.IsDevelopmentMode,
		ScalingSpec:          hibernation(src),
		ResourceQuotaCpu:     src.ResourceQuotaCpu,
		ResourceQuotaMemory:  src.ResourceQuotaMemory,
		DefaultTaskPodCpu:    src.DefaultTaskPodCpu,
		DefaultTaskPodMemory: src.DefaultTaskPodMemory,
		WorkerQueues:         queues(src),
	}
	if c.executor != nil {
		r.Executor = new(astrov1.CreateStandardDeploymentRequestExecutor(*c.executor))
	}
	if src.SchedulerSize != nil {
		r.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSize(*src.SchedulerSize))
	}
	return r, nil
}

func dedicated(src *astrov1.Deployment, c *fields) (astrov1.CreateDedicatedDeploymentRequest, []Note, error) {
	if src.ClusterId == nil || *src.ClusterId == "" {
		return astrov1.CreateDedicatedDeploymentRequest{}, nil, errors.New("the Deployment has no cluster, so it cannot be copied")
	}
	r := astrov1.CreateDedicatedDeploymentRequest{
		Name:                 c.name,
		WorkspaceId:          c.workspaceID,
		Type:                 new(astrov1.CreateDedicatedDeploymentRequestTypeDEDICATED),
		AstroRuntimeVersion:  &c.runtimeVersion,
		Description:          c.description,
		IsCicdEnforced:       &c.isCicdEnforced,
		IsDagDeployEnabled:   &c.isDagDeployOn,
		ContactEmails:        c.contactEmails,
		EnvironmentVariables: c.env,
		ClusterId:            new(*src.ClusterId),
		IsHighAvailability:   src.IsHighAvailability,
		IsDevelopmentMode:    src.IsDevelopmentMode,
		ScalingSpec:          hibernation(src),
	}
	if c.executor != nil {
		r.Executor = new(astrov1.CreateDedicatedDeploymentRequestExecutor(*c.executor))
	}
	if src.SchedulerSize != nil {
		r.SchedulerSize = new(astrov1.CreateDedicatedDeploymentRequestSchedulerSize(*src.SchedulerSize))
	}
	var notes []Note
	if re := src.RemoteExecution; re != nil && re.Enabled {
		// Remote Execution excludes the resource quotas and task pod sizes,
		// and runs no worker queues on Astro.
		r.RemoteExecution = &astrov1.DeploymentRemoteExecutionRequest{
			Enabled:                true,
			AllowedIpAddressRanges: new(append([]string{}, re.AllowedIpAddressRanges...)),
			TaskLogBucket:          re.TaskLogBucket,
			TaskLogUrlPattern:      re.TaskLogUrlPattern,
		}
		notes = append(notes, "Not copied: Remote Execution agents. Register agents for the new Deployment before it runs tasks.")
		return r, notes, nil
	}
	// With Remote Execution off, no remoteExecution is sent at all: one
	// disabled and no queues is refused as "worker queues are required" even
	// for an executor that has none.
	r.ResourceQuotaCpu = src.ResourceQuotaCpu
	r.ResourceQuotaMemory = src.ResourceQuotaMemory
	r.DefaultTaskPodCpu = src.DefaultTaskPodCpu
	r.DefaultTaskPodMemory = src.DefaultTaskPodMemory
	r.WorkerQueues = queues(src)
	return r, notes, nil
}

func hybrid(src *astrov1.Deployment, c *fields) (astrov1.CreateHybridDeploymentRequest, error) {
	if src.ClusterId == nil || *src.ClusterId == "" {
		return astrov1.CreateHybridDeploymentRequest{}, errors.New("the Deployment has no cluster, so it cannot be copied")
	}
	r := astrov1.CreateHybridDeploymentRequest{
		Name:                 c.name,
		WorkspaceId:          c.workspaceID,
		Type:                 new(astrov1.CreateHybridDeploymentRequestTypeHYBRID),
		AstroRuntimeVersion:  &c.runtimeVersion,
		Description:          c.description,
		IsCicdEnforced:       &c.isCicdEnforced,
		IsDagDeployEnabled:   &c.isDagDeployOn,
		ContactEmails:        c.contactEmails,
		EnvironmentVariables: c.env,
		ClusterId:            new(*src.ClusterId),
	}
	if c.executor != nil {
		r.Executor = new(astrov1.CreateHybridDeploymentRequestExecutor(*c.executor))
	}
	if src.SchedulerAu != nil || src.SchedulerReplicas != 0 {
		r.Scheduler = &astrov1.CreateDeploymentInstanceSpecRequest{Au: src.SchedulerAu}
		if src.SchedulerReplicas != 0 {
			r.Scheduler.Replicas = new(src.SchedulerReplicas)
		}
	}
	if src.Executor != nil && *src.Executor == astrov1.DeploymentExecutorKUBERNETES {
		r.TaskPodNodePoolId = src.TaskPodNodePoolId
		return r, nil
	}
	if src.WorkerQueues != nil && runsQueues(src) {
		qs := []astrov1.HybridWorkerQueueRequest{}
		for _, q := range *src.WorkerQueues {
			hq := astrov1.HybridWorkerQueueRequest{
				Name:              q.Name,
				IsDefault:         q.IsDefault,
				MinWorkerCount:    q.MinWorkerCount,
				MaxWorkerCount:    q.MaxWorkerCount,
				WorkerConcurrency: q.WorkerConcurrency,
			}
			if q.NodePoolId != nil {
				hq.NodePoolId = *q.NodePoolId
			}
			qs = append(qs, hq)
		}
		r.WorkerQueues = &qs
	}
	return r, nil
}

// runsQueues reports whether src's executor runs worker queues. A Kubernetes
// Deployment has none, and the GET returns none for it.
func runsQueues(src *astrov1.Deployment) bool {
	return src.Executor == nil || *src.Executor != astrov1.DeploymentExecutorKUBERNETES
}

// queues copies a hosted Deployment's worker queues: each one's shape, not
// its id, and not the pod CPU and memory its machine decides.
func queues(src *astrov1.Deployment) *[]astrov1.WorkerQueueRequest {
	if src.WorkerQueues == nil || len(*src.WorkerQueues) == 0 || !runsQueues(src) {
		return nil
	}
	qs := []astrov1.WorkerQueueRequest{}
	for _, q := range *src.WorkerQueues {
		wq := astrov1.WorkerQueueRequest{
			Name:                q.Name,
			IsDefault:           q.IsDefault,
			MinWorkerCount:      q.MinWorkerCount,
			MaxWorkerCount:      q.MaxWorkerCount,
			WorkerConcurrency:   q.WorkerConcurrency,
			PodEphemeralStorage: q.PodEphemeralStorage,
		}
		if q.AstroMachine != nil {
			wq.AstroMachine = astrov1.WorkerQueueRequestAstroMachine(*q.AstroMachine)
		}
		qs = append(qs, wq)
	}
	return &qs
}

// hibernation copies a development Deployment's hibernation schedules. Only a
// development Deployment may have them, and its override is the state of the
// moment, not configuration, so it is never copied.
func hibernation(src *astrov1.Deployment) *astrov1.DeploymentScalingSpecRequest {
	if src.IsDevelopmentMode == nil || !*src.IsDevelopmentMode {
		return nil
	}
	if src.ScalingSpec == nil || src.ScalingSpec.HibernationSpec == nil || src.ScalingSpec.HibernationSpec.Schedules == nil {
		return nil
	}
	schedules := append([]astrov1.DeploymentHibernationSchedule{}, *src.ScalingSpec.HibernationSpec.Schedules...)
	return &astrov1.DeploymentScalingSpecRequest{
		HibernationSpec: &astrov1.DeploymentHibernationSpecRequest{Schedules: &schedules},
	}
}
