package clone

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

func ptr[T any](v T) *T { return &v }

// The source fixtures are Deployments as the API's GET returns them, with the
// fields a copy must not carry (ids, status, URLs, image, names of things)
// filled in, so a request that leaked one would show it.

func base(typ astrov1.DeploymentType, executor astrov1.DeploymentExecutor) astrov1.Deployment {
	return astrov1.Deployment{
		Id:                  "clsrc00000000000000000001",
		Name:                "etl-prod",
		Namespace:           "fiery-nebula-1234",
		OrganizationId:      "org-id",
		WorkspaceId:         "ws-src",
		WorkspaceName:       ptr("Data"),
		AstroRuntimeVersion: "13.1.0",
		RuntimeVersion:      "13.1.0",
		AirflowVersion:      "3.0.4",
		ImageRepository:     "images.astronomer.cloud/x",
		ImageTag:            "deploy-2026",
		Status:              astrov1.DeploymentStatusHEALTHY,
		Type:                ptr(typ),
		Executor:            ptr(executor),
		Description:         ptr("Nightly ETL"),
		IsCicdEnforced:      true,
		IsDagDeployEnabled:  true,
		ContactEmails:       &[]string{"oncall@example.com"},
		WebServerUrl:        "etl.astronomer.run/d1",
		UiUrl:               "etl.astronomer.run/d1",
		CreatedAt:           time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		EnvironmentVariables: &[]astrov1.DeploymentEnvironmentVariable{
			{Key: "PLAIN", Value: ptr("1")},
		},
		EffectiveWorkloadIdentity: ptr("arn:aws:iam::123456789012:role/astro-fiery-nebula-1234"),
	}
}

func hostedResources(d *astrov1.Deployment) {
	d.SchedulerSize = ptr(astrov1.DeploymentSchedulerSizeMEDIUM)
	d.IsHighAvailability = ptr(true)
	d.IsDevelopmentMode = ptr(false)
	d.ResourceQuotaCpu = ptr("10")
	d.ResourceQuotaMemory = ptr("20Gi")
	d.DefaultTaskPodCpu = ptr("0.25")
	d.DefaultTaskPodMemory = ptr("0.5Gi")
}

func celeryQueues() *[]astrov1.WorkerQueue {
	return &[]astrov1.WorkerQueue{
		{Id: "q1", Name: "default", IsDefault: true, MinWorkerCount: 1, MaxWorkerCount: 10, WorkerConcurrency: 5, AstroMachine: ptr("A5"), PodCpu: "1", PodMemory: "2Gi", PodEphemeralStorage: ptr("10Gi")},
		{Id: "q2", Name: "heavy", MinWorkerCount: 0, MaxWorkerCount: 2, WorkerConcurrency: 1, AstroMachine: ptr("A20"), PodCpu: "4", PodMemory: "8Gi"},
	}
}

func standardCelery() astrov1.Deployment {
	d := base(astrov1.DeploymentTypeSTANDARD, astrov1.DeploymentExecutorCELERY)
	d.CloudProvider = ptr(astrov1.DeploymentCloudProviderAWS)
	d.Region = ptr("us-east-1")
	d.ClusterName = ptr("shared-aws")
	hostedResources(&d)
	d.WorkerQueues = celeryQueues()
	return d
}

func dedicatedAstro() astrov1.Deployment {
	d := base(astrov1.DeploymentTypeDEDICATED, astrov1.DeploymentExecutorASTRO)
	d.ClusterId = ptr("clcluster0000000000000001")
	d.ClusterName = ptr("my-cluster")
	d.CloudProvider = ptr(astrov1.DeploymentCloudProviderAWS)
	d.Region = ptr("us-east-1")
	hostedResources(&d)
	d.WorkerQueues = celeryQueues()
	return d
}

func hybridBase(executor astrov1.DeploymentExecutor) astrov1.Deployment {
	d := base(astrov1.DeploymentTypeHYBRID, executor)
	d.ClusterId = ptr("clhybrid00000000000000001")
	d.ClusterName = ptr("byoc")
	d.CloudProvider = ptr(astrov1.DeploymentCloudProviderAWS)
	d.Region = ptr("us-east-1")
	d.SchedulerAu = ptr(10)
	d.SchedulerReplicas = 2
	d.EffectiveWorkloadIdentity = ptr("arn:aws:iam::123456789012:role/AirflowS3Logs-clhybrid00000000000000001")
	return d
}

const commonJSON = `"name":"etl-preview","workspaceId":"ws-src","astroRuntimeVersion":"13.1.0",
	"description":"Nightly ETL","isCicdEnforced":true,"isDagDeployEnabled":true,
	"contactEmails":["oncall@example.com"],
	"environmentVariables":[{"key":"PLAIN","value":"1","isSecret":false}]`

const hostedResourcesJSON = `"schedulerSize":"MEDIUM","isHighAvailability":true,"isDevelopmentMode":false,
	"resourceQuotaCpu":"10","resourceQuotaMemory":"20Gi","defaultTaskPodCpu":"0.25","defaultTaskPodMemory":"0.5Gi"`

const queuesJSON = `"workerQueues":[
	{"name":"default","isDefault":true,"minWorkerCount":1,"maxWorkerCount":10,"workerConcurrency":5,"astroMachine":"A5","podEphemeralStorage":"10Gi"},
	{"name":"heavy","isDefault":false,"minWorkerCount":0,"maxWorkerCount":2,"workerConcurrency":1,"astroMachine":"A20"}]`

func TestRequest(t *testing.T) {
	cases := []struct {
		name        string
		src         func() astrov1.Deployment
		workspace   string
		description *string
		want        string
		notes       []string
	}{
		{
			name: "standard celery",
			src:  standardCelery,
			want: `{"type":"STANDARD","executor":"CELERY",` + commonJSON + `,
				"cloudProvider":"AWS","region":"us-east-1",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
		},
		{
			name: "standard kubernetes sends no queues",
			src: func() astrov1.Deployment {
				d := standardCelery()
				d.Executor = ptr(astrov1.DeploymentExecutorKUBERNETES)
				d.WorkerQueues = nil
				return d
			},
			want: `{"type":"STANDARD","executor":"KUBERNETES",` + commonJSON + `,
				"cloudProvider":"AWS","region":"us-east-1",` + hostedResourcesJSON + `}`,
		},
		{
			name: "a development Deployment keeps its schedules and drops its override",
			src: func() astrov1.Deployment {
				d := standardCelery()
				d.IsDevelopmentMode = ptr(true)
				d.IsHighAvailability = ptr(false)
				d.ScalingSpec = &astrov1.DeploymentScalingSpec{HibernationSpec: &astrov1.DeploymentHibernationSpec{
					Schedules: &[]astrov1.DeploymentHibernationSchedule{{HibernateAtCron: "0 20 * * *", WakeAtCron: "0 8 * * *", IsEnabled: true, Description: ptr("nights")}},
					Override:  &astrov1.DeploymentHibernationOverride{IsHibernating: ptr(true), IsActive: ptr(true)},
				}}
				return d
			},
			want: `{"type":"STANDARD","executor":"CELERY",` + commonJSON + `,
				"cloudProvider":"AWS","region":"us-east-1",
				"schedulerSize":"MEDIUM","isHighAvailability":false,"isDevelopmentMode":true,
				"resourceQuotaCpu":"10","resourceQuotaMemory":"20Gi","defaultTaskPodCpu":"0.25","defaultTaskPodMemory":"0.5Gi",
				"scalingSpec":{"hibernationSpec":{"schedules":[{"hibernateAtCron":"0 20 * * *","wakeAtCron":"0 8 * * *","isEnabled":true,"description":"nights"}]}},` + queuesJSON + `}`,
		},
		{
			name: "schedules on a Deployment not in development mode are not sent",
			src: func() astrov1.Deployment {
				d := standardCelery()
				d.ScalingSpec = &astrov1.DeploymentScalingSpec{HibernationSpec: &astrov1.DeploymentHibernationSpec{
					Schedules: &[]astrov1.DeploymentHibernationSchedule{{HibernateAtCron: "0 20 * * *", WakeAtCron: "0 8 * * *", IsEnabled: true}},
				}}
				return d
			},
			want: `{"type":"STANDARD","executor":"CELERY",` + commonJSON + `,
				"cloudProvider":"AWS","region":"us-east-1",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
		},
		{
			name: "dedicated without remote execution",
			src:  dedicatedAstro,
			want: `{"type":"DEDICATED","executor":"ASTRO",` + commonJSON + `,
				"clusterId":"clcluster0000000000000001",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
		},
		{
			name: "dedicated with remote execution sends no quotas, task pod sizes or queues",
			src: func() astrov1.Deployment {
				d := dedicatedAstro()
				d.WorkerQueues = &[]astrov1.WorkerQueue{}
				d.RemoteExecution = &astrov1.DeploymentRemoteExecution{
					Enabled:                true,
					AllowedIpAddressRanges: []string{"203.0.113.0/24"},
					RemoteApiUrl:           "https://remote.example/api",
					TaskLogBucket:          ptr("s3://logs"),
					TaskLogUrlPattern:      ptr("{{ ti.dag_id }}"),
				}
				return d
			},
			want: `{"type":"DEDICATED","executor":"ASTRO",` + commonJSON + `,
				"clusterId":"clcluster0000000000000001",
				"schedulerSize":"MEDIUM","isHighAvailability":true,"isDevelopmentMode":false,
				"remoteExecution":{"enabled":true,"allowedIpAddressRanges":["203.0.113.0/24"],"taskLogBucket":"s3://logs","taskLogUrlPattern":"{{ ti.dag_id }}"}}`,
			notes: []string{"Not copied: Remote Execution agents. Register agents for the new Deployment before it runs tasks."},
		},
		{
			name: "dedicated with remote execution disabled sends none",
			src: func() astrov1.Deployment {
				d := dedicatedAstro()
				d.RemoteExecution = &astrov1.DeploymentRemoteExecution{Enabled: false, AllowedIpAddressRanges: []string{}}
				return d
			},
			want: `{"type":"DEDICATED","executor":"ASTRO",` + commonJSON + `,
				"clusterId":"clcluster0000000000000001",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
		},
		{
			name: "hybrid celery copies each queue's node pool",
			src: func() astrov1.Deployment {
				d := hybridBase(astrov1.DeploymentExecutorCELERY)
				d.WorkerQueues = &[]astrov1.WorkerQueue{
					{Id: "q1", Name: "default", IsDefault: true, MinWorkerCount: 1, MaxWorkerCount: 10, WorkerConcurrency: 16, NodePoolId: ptr("np-1"), PodCpu: "1", PodMemory: "2Gi"},
				}
				return d
			},
			want: `{"type":"HYBRID","executor":"CELERY",` + commonJSON + `,
				"clusterId":"clhybrid00000000000000001","scheduler":{"au":10,"replicas":2},
				"workerQueues":[{"name":"default","isDefault":true,"minWorkerCount":1,"maxWorkerCount":10,"workerConcurrency":16,"nodePoolId":"np-1"}]}`,
		},
		{
			name: "hybrid kubernetes copies the task pod node pool",
			src: func() astrov1.Deployment {
				d := hybridBase(astrov1.DeploymentExecutorKUBERNETES)
				d.TaskPodNodePoolId = ptr("np-2")
				return d
			},
			want: `{"type":"HYBRID","executor":"KUBERNETES",` + commonJSON + `,
				"clusterId":"clhybrid00000000000000001","scheduler":{"au":10,"replicas":2},
				"taskPodNodePoolId":"np-2"}`,
		},
		{
			name: "secret variables are left out and named",
			src: func() astrov1.Deployment {
				d := standardCelery()
				d.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
					{Key: "PLAIN", Value: ptr("1")},
					{Key: "API_KEY", IsSecret: true},
					{Key: "DB_PASSWORD", IsSecret: true},
				}
				return d
			},
			want: `{"type":"STANDARD","executor":"CELERY",` + commonJSON + `,
				"cloudProvider":"AWS","region":"us-east-1",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
			notes: []string{"Not copied: secret environment variables API_KEY, DB_PASSWORD. The API does not return their values; set them on the new Deployment with `astro deployment variable create --secret`."},
		},
		{
			name: "a custom-looking workload identity is named, not sent",
			src: func() astrov1.Deployment {
				d := standardCelery()
				d.EffectiveWorkloadIdentity = ptr("arn:aws:iam::123456789012:role/my-etl-role")
				return d
			},
			want: `{"type":"STANDARD","executor":"CELERY",` + commonJSON + `,
				"cloudProvider":"AWS","region":"us-east-1",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
			notes: []string{"Not copied: the workload identity arn:aws:iam::123456789012:role/my-etl-role, which looks custom. The new Deployment has Astro's default; set it with `astro deployment update --workload-identity` if it should match."},
		},
		{
			name: "a dedicated Deployment on its cluster's role has a custom identity",
			src: func() astrov1.Deployment {
				d := dedicatedAstro()
				d.EffectiveWorkloadIdentity = ptr("arn:aws:iam::123456789012:role/AirflowS3Logs-clcluster0000000000000001")
				return d
			},
			want: `{"type":"DEDICATED","executor":"ASTRO",` + commonJSON + `,
				"clusterId":"clcluster0000000000000001",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
			notes: []string{"Not copied: the workload identity arn:aws:iam::123456789012:role/AirflowS3Logs-clcluster0000000000000001, which looks custom. The new Deployment has Astro's default; set it with `astro deployment update --workload-identity` if it should match."},
		},
		{
			name: "a hybrid Deployment on a per-Deployment role has a custom identity",
			src: func() astrov1.Deployment {
				d := hybridBase(astrov1.DeploymentExecutorKUBERNETES)
				d.EffectiveWorkloadIdentity = ptr("arn:aws:iam::123456789012:role/astro-fiery-nebula-1234")
				return d
			},
			want: `{"type":"HYBRID","executor":"KUBERNETES",` + commonJSON + `,
				"clusterId":"clhybrid00000000000000001","scheduler":{"au":10,"replicas":2}}`,
			notes: []string{"Not copied: the workload identity arn:aws:iam::123456789012:role/astro-fiery-nebula-1234, which looks custom. The new Deployment has Astro's default; set it with `astro deployment update --workload-identity` if it should match."},
		},
		{
			name: "a custom disaster recovery identity is named, with no CLI flag offered",
			src: func() astrov1.Deployment {
				d := standardCelery()
				d.EffectiveDRWorkloadIdentity = ptr("arn:aws:iam::210987654321:role/etl-dr")
				return d
			},
			want: `{"type":"STANDARD","executor":"CELERY",` + commonJSON + `,
				"cloudProvider":"AWS","region":"us-east-1",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
			notes: []string{"Not copied: the disaster recovery workload identity arn:aws:iam::210987654321:role/etl-dr, which looks custom. The new Deployment has Astro's default, and the CLI cannot set this one: set it in the Astro UI or with the API if it should match."},
		},
		{
			name: "--workspace and --description replace the source's",
			src: func() astrov1.Deployment {
				d := standardCelery()
				// The DR side's default is the same role as the primary's.
				d.EffectiveDRWorkloadIdentity = ptr("arn:aws:iam::210987654321:role/astro-fiery-nebula-1234")
				return d
			},
			workspace:   "ws-other",
			description: ptr("Preview of feature-x"),
			want: `{"type":"STANDARD","executor":"CELERY","name":"etl-preview","workspaceId":"ws-other","astroRuntimeVersion":"13.1.0",
				"description":"Preview of feature-x","isCicdEnforced":true,"isDagDeployEnabled":true,
				"contactEmails":["oncall@example.com"],
				"environmentVariables":[{"key":"PLAIN","value":"1","isSecret":false}],
				"cloudProvider":"AWS","region":"us-east-1",` + hostedResourcesJSON + `,` + queuesJSON + `}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src()
			req, notes, err := Request(&src, "etl-preview", tc.workspace, tc.description)
			require.NoError(t, err)
			got, err := json.Marshal(req)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))
			var gotNotes []string
			for _, n := range notes {
				gotNotes = append(gotNotes, string(n))
			}
			assert.Equal(t, tc.notes, gotNotes)
		})
	}
}

// The identity Astro gives a Deployment of its own is the source's, not one
// to copy, and is not worth a note. Which identity that is depends on the
// cloud and the type, as core's DefaultWorkloadIdentity decides: a pattern
// that is another type's default, or another Deployment's, is custom.
func TestIsDefaultIdentity(t *testing.T) {
	const (
		ns          = "a-very-long-release-name-0123456789"
		cluster     = "clhybrid00000000000000001"
		ownRole     = "arn:aws:iam::123456789012:role/astro-" + ns
		clusterRole = "arn:aws:iam::123456789012:role/AirflowS3Logs-" + cluster
		gcpDefault  = "astro-a-very-long-release-name@proj.iam.gserviceaccount.com"
		azureID     = "/subscriptions/x/resourceGroups/y/providers/Microsoft.ManagedIdentity/etl-id"
	)
	aws, gcp, azure := astrov1.DeploymentCloudProviderAWS, astrov1.DeploymentCloudProviderGCP, astrov1.DeploymentCloudProviderAZURE
	std, ded, hyb := astrov1.DeploymentTypeSTANDARD, astrov1.DeploymentTypeDEDICATED, astrov1.DeploymentTypeHYBRID
	cases := []struct {
		cloud astrov1.DeploymentCloudProvider
		typ   astrov1.DeploymentType
		id    string
		want  bool
	}{
		// Each type's true default.
		{aws, std, ownRole, true},
		{aws, ded, ownRole, true},
		{aws, hyb, clusterRole, true},
		{gcp, std, gcpDefault, true},
		{gcp, ded, gcpDefault, true},
		{gcp, hyb, gcpDefault, true},
		{azure, ded, "", true},
		// Another type's default is custom here.
		{aws, ded, clusterRole, false},
		{aws, std, clusterRole, false},
		{aws, hyb, ownRole, false},
		// Another Deployment's or cluster's is custom.
		{aws, std, "arn:aws:iam::123456789012:role/astro-another-deployment", false},
		{aws, hyb, "arn:aws:iam::123456789012:role/AirflowS3Logs-clother0000000000000000001", false},
		{gcp, std, "astro-a-very-long-release-name-0123456789@proj.iam.gserviceaccount.com", false},
		{gcp, std, "etl@proj.iam.gserviceaccount.com", false},
		// Another cloud's pattern is custom, and Azure has no default.
		{gcp, std, ownRole, false},
		{aws, std, gcpDefault, false},
		{azure, ded, azureID, false},
		{azure, ded, ownRole, false},
		{azure, ded, gcpDefault, false},
	}
	for _, tc := range cases {
		src := standardCelery()
		src.Namespace, src.ClusterId = ns, ptr(cluster)
		src.CloudProvider, src.Type = ptr(tc.cloud), ptr(tc.typ)
		assert.Equal(t, tc.want, isDefaultIdentity(&src, tc.id), "%s %s %s", tc.cloud, tc.typ, tc.id)
	}
}

func TestRequestRefuses(t *testing.T) {
	noType := standardCelery()
	noType.Type = nil
	noRegion := standardCelery()
	noRegion.Region = nil
	noCluster := dedicatedAstro()
	noCluster.ClusterId = nil
	for name, tc := range map[string]struct {
		src     astrov1.Deployment
		newName string
		want    string
	}{
		"no name":                   {standardCelery(), "", "needs a name"},
		"no type":                   {noType, "x", "no type"},
		"standard with no region":   {noRegion, "x", "no cloud provider or region"},
		"dedicated with no cluster": {noCluster, "x", "no cluster"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := Request(&tc.src, tc.newName, "", nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
