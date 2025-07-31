package houston

import (
	"time"
)

// ListDeploymentsRequest - filters to list deployments according to set values
type ListDeploymentsRequest struct {
	WorkspaceID string `json:"workspaceId"`
	ReleaseName string `json:"releaseName"`
}

type PaginatedDeploymentsRequest struct {
	Take int `json:"take"`
}

// ListDeploymentLogsRequest - filters to list logs from a deployment
type ListDeploymentLogsRequest struct {
	DeploymentID string    `json:"deploymentId"`
	Component    string    `json:"component"`
	Search       string    `json:"search"`
	Timestamp    time.Time `json:"timestamp"`
}

// UpdateDeploymentImageRequest - properties to update a deployment image
type UpdateDeploymentImageRequest struct {
	ReleaseName    string `json:"releaseName"`
	Image          string `json:"image"`
	AirflowVersion string `json:"airflowVersion"`
	RuntimeVersion string `json:"runtimeVersion"`
}

// DeleteDeploymentRequest - properties to delete a deployment
type DeleteDeploymentRequest struct {
	DeploymentID string `json:"deploymentId"`
	HardDelete   bool   `json:"deploymentHardDelete"`
}

var (
	DeploymentCreateRequest = queryList{
		{
			version: "0.25.0",
			query: `
			mutation CreateDeployment(
				$label: String!
				$type: String = "airflow"
				$releaseName: String
				$workspaceId: Uuid!
				$executor: ExecutorType!
				$airflowVersion: String
				$namespace: String
				$config: JSON
				$cloudRole: String
				$dagDeployment: DagDeployment
			) {
				createDeployment(
					label: $label
					type: $type
					workspaceUuid: $workspaceId
					releaseName: $releaseName
					executor: $executor
					airflowVersion: $airflowVersion
					namespace: $namespace
					config: $config
					cloudRole: $cloudRole
					dagDeployment: $dagDeployment
				) {
					id
					type
					label
					releaseName
					version
					airflowVersion
					urls {
						type
						url
					}
					createdAt
					updatedAt
				}
			}`,
		},
		{
			version: "0.28.0",
			query: `
			mutation CreateDeployment(
				$label: String!
				$type: String = "airflow"
				$releaseName: String
				$workspaceId: Uuid!
				$executor: ExecutorType!
				$airflowVersion: String
				$namespace: String
				$config: JSON
				$cloudRole: String
				$dagDeployment: DagDeployment
				$triggererReplicas: Int
			) {
				createDeployment(
					label: $label
					type: $type
					workspaceUuid: $workspaceId
					releaseName: $releaseName
					executor: $executor
					airflowVersion: $airflowVersion
					namespace: $namespace
					config: $config
					cloudRole: $cloudRole
					dagDeployment: $dagDeployment
					triggerer: {
						replicas: $triggererReplicas
					}
				) {
					id
					type
					label
					releaseName
					version
					airflowVersion
					urls {
						type
						url
					}
					createdAt
					updatedAt
				}
			}`,
		},
		{
			version: "0.29.0",
			query: `
			mutation CreateDeployment(
				$label: String!
				$type: String = "airflow"
				$releaseName: String
				$workspaceId: Uuid!
				$executor: ExecutorType!
				$airflowVersion: String
				$runtimeVersion: String
				$namespace: String
				$config: JSON
				$cloudRole: String
				$dagDeployment: DagDeployment
				$triggererReplicas: Int
			){
				createDeployment(
					label: $label
					type: $type
					workspaceUuid: $workspaceId
					releaseName: $releaseName
					executor: $executor
					airflowVersion: $airflowVersion
					runtimeVersion: $runtimeVersion
					namespace: $namespace
					config: $config
					cloudRole: $cloudRole
					dagDeployment: $dagDeployment
					triggerer: {
						replicas: $triggererReplicas
					}
				){
					id
					type
					label
					releaseName
					version
					airflowVersion
					runtimeVersion
					urls {
						type
						url
					}
					createdAt
					updatedAt
				}
			}`,
		},
	}

	DeploymentsGetRequest = queryList{
		{
			version: "0.25.0",
			query: `
			query GetDeployment(
				$workspaceId: Uuid!
				$releaseName: String
			) {
				workspaceDeployments(
					workspaceUuid: $workspaceId
					releaseName: $releaseName
				) {
					id
					type
					label
					releaseName
					workspace {
						id
					}
					deployInfo {
						nextCli
						current
					}
					version
					airflowVersion
					createdAt
					updatedAt
				}
			}`,
		},
		{
			version: "0.29.0",
			query: `
			query GetDeployment(
				$workspaceId: Uuid!
				$releaseName: String
			){
				workspaceDeployments(
					workspaceUuid: $workspaceId
					releaseName: $releaseName
				){
					id
					type
					label
					releaseName
					workspace {
						id
					}
					deployInfo {
						nextCli
						current
					}
					version
					airflowVersion
					runtimeVersion
					createdAt
					updatedAt
				}
			}`,
		},
        {
			version: "1.0.0",
			query: `
			query GetDeployment(
				$workspaceId: Uuid!
				$releaseName: String
			){
				workspaceDeployments(
					workspaceUuid: $workspaceId
					releaseName: $releaseName
				){
					id
					type
					label
					releaseName
					workspace {
						id
					}
					deployInfo {
						nextCli
						current
					}
					version
					airflowVersion
					runtimeVersion
					clusterId
					createdAt
					updatedAt
				}
			}`,
		},
	}

	PaginatedDeploymentsGetRequest = queryList{
		{
			version: "0.32.0",
			query: `
			query paginatedDeployments( $take: Int, $name: String, $cursor: Uuid, $pageNumber: Int) {
				paginatedDeployments(
					take: $take
					name: $name
					cursor: $cursor
					pageNumber: $pageNumber
				) {
					id
					type
					label
					releaseName
					workspace {
						id
					}
					deployInfo {
						nextCli
						current
					}
					version
					airflowVersion
					runtimeVersion
					createdAt
					updatedAt
				}
			}`,
		},
	}

	DeploymentUpdateRequest = queryList{
		{
			version: "0.25.0",
			query: `
			mutation UpdateDeployment(
				$deploymentId: Uuid!,
				$payload: JSON!,
				$cloudRole: String,
				$executor: ExecutorType,
				$dagDeployment: DagDeployment
			){
				updateDeployment(
					deploymentUuid: $deploymentId,
					payload: $payload,
					cloudRole: $cloudRole,
					executor: $executor,
					dagDeployment: $dagDeployment
				){
					id
					type
					label
					description
					releaseName
					version
					airflowVersion
					workspace {
						id
					}
					deployInfo {
						current
					}
					createdAt
					updatedAt
				}
			}`,
		},
		{
			version: "0.28.0",
			query: `
			mutation UpdateDeployment(
				$deploymentId: Uuid!,
				$payload: JSON!,
				$executor: ExecutorType,
				$cloudRole: String,
				$dagDeployment: DagDeployment,
				$triggererReplicas: Int
			){
				updateDeployment(
					deploymentUuid: $deploymentId,
					payload: $payload,
					executor: $executor,
					cloudRole: $cloudRole,
					dagDeployment: $dagDeployment,
					triggerer:{ replicas: $triggererReplicas }
				){
					id
					type
					label
					description
					releaseName
					version
					airflowVersion
					workspace {
						id
					}
					deployInfo {
						current
					}
					createdAt
					updatedAt
				}
			}`,
		},
	}

	DeploymentGetRequest = queryList{
		{
			version: "0.25.0",
			query: `
			query GetDeployment(
				$id: String!
			) {
				deployment(
					where: {id: $id}
				) {
					id
					airflowVersion
					desiredAirflowVersion
					urls {
						type
						url
					}
				}
			}`,
		},
		{
			version: "0.29.0",
			query: `
			query GetDeployment(
				$id: String!
			){
				deployment(
					where: {id: $id}
				){
					id
					airflowVersion
					desiredAirflowVersion
					runtimeVersion
					desiredRuntimeVersion
					runtimeAirflowVersion
					releaseName
					urls {
						type
						url
					}
					dagDeployment {
						type
					}
				}
			}`,
		},
		{
			version: "1.0.0",
			query: `
			query GetDeployment(
				$id: String!
			){
				deployment(
					where: {id: $id}
				){
					id
					airflowVersion
					desiredAirflowVersion
					runtimeVersion
					desiredRuntimeVersion
					runtimeAirflowVersion
					releaseName
					urls {
						type
						url
					}
					dagDeployment {
						type
					}
					clusterId
				}
			}`,
		},
	}

	DeploymentDeleteRequest = `
	mutation DeleteDeployment(
		$deploymentId: Uuid!
		$deploymentHardDelete: Boolean
	){
		deleteDeployment(
			deploymentUuid: $deploymentId
			deploymentHardDelete: $deploymentHardDelete
		){
			id
			type
			label
			description
			releaseName
			version
			workspace {
				id
			}
			createdAt
			updatedAt
		}
	}`

	UpdateDeploymentAirflowRequest = `
	mutation updateDeploymentAirflow($deploymentId: Uuid!, $desiredAirflowVersion: String!) {
		updateDeploymentAirflow(deploymentUuid: $deploymentId, desiredAirflowVersion: $desiredAirflowVersion) {
			id
			label
			version
			releaseName
			airflowVersion
			desiredAirflowVersion
		}
	}`

	DeploymentInfoRequest = `
	query DeploymentInfo {
		deploymentConfig {
			airflowImages {
				version
				tag
			}
			airflowVersions
			defaultAirflowImageTag
		}
	}`

	DeploymentLogsGetRequest = `
	query GetLogs(
		$deploymentId: Uuid!
		$component: String
		$timestamp: DateTime
		$search: String
	){
		logs(
			deploymentUuid: $deploymentId
			component: $component
			timestamp: $timestamp
			search: $search
		){
			id
			createdAt: timestamp
			log: message
		}
	}`

	DeploymentImageUpdateRequest = `
	mutation updateDeploymentImage(
		$releaseName:String!,
		$image:String!,
		$airflowVersion:String,
		$runtimeVersion:String,
	){
		updateDeploymentImage(
			releaseName:$releaseName,
			image:$image,
			airflowVersion:$airflowVersion,
			runtimeVersion:$runtimeVersion
		){
			releaseName
			airflowVersion
			runtimeVersion
		}
	}`

	UpdateDeploymentRuntimeRequest = `
	mutation updateDeploymentRuntime($deploymentUuid: Uuid!, $desiredRuntimeVersion: String!) {
		updateDeploymentRuntime(deploymentUuid: $deploymentUuid, desiredRuntimeVersion: $desiredRuntimeVersion) {
		  	id
			label
			version
			releaseName
		  	runtimeVersion
		  	desiredRuntimeVersion
		  	runtimeAirflowVersion
		}
	}`

	CancelUpdateDeploymentRuntimeRequest = `
	mutation cancelRuntimeUpdate($deploymentUuid: Uuid!) {
		cancelRuntimeUpdate(deploymentUuid: $deploymentUuid) {
			id
			label
			version
			releaseName
			runtimeVersion
			desiredRuntimeVersion
			runtimeAirflowVersion
		}
	}`
)

// CreateDeployment - create a deployment
func (h ClientImplementation) CreateDeployment(vars map[string]interface{}) (*Deployment, error) {
	reqQuery := DeploymentCreateRequest.GreatestLowerBound(version)
	req := Request{
		Query:     reqQuery,
		Variables: vars,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.CreateDeployment, nil
}

// DeleteDeployment - delete a deployment
func (h ClientImplementation) DeleteDeployment(request DeleteDeploymentRequest) (*Deployment, error) {
	req := Request{
		Query:     DeploymentDeleteRequest,
		Variables: request,
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return res.Data.DeleteDeployment, nil
}

// ListDeployments - List deployments from the API
func (h ClientImplementation) ListDeployments(filters ListDeploymentsRequest) ([]Deployment, error) {
	variables := map[string]interface{}{}
	if filters.WorkspaceID != "" {
		variables["workspaceId"] = filters.WorkspaceID
	}
	if filters.ReleaseName != "" {
		variables["releaseName"] = filters.ReleaseName
	}

	reqQuery := DeploymentsGetRequest.GreatestLowerBound(version)
	req := Request{
		Query: reqQuery,
	}

	if len(variables) > 0 {
		req.Variables = variables
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return res.Data.GetDeployments, nil
}

func (h ClientImplementation) ListPaginatedDeployments(filters PaginatedDeploymentsRequest) ([]Deployment, error) {
	variables := map[string]interface{}{}
	variables["take"] = filters.Take
	reqQuery := PaginatedDeploymentsGetRequest.GreatestLowerBound(version)
	req := Request{
		Query: reqQuery,
	}
	if len(variables) > 0 {
		req.Variables = variables
	}
	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}
	return res.Data.PaginatedDeployments, nil
}

// UpdateDeployment - update a deployment
func (h ClientImplementation) UpdateDeployment(variables map[string]interface{}) (*Deployment, error) {
	reqQuery := DeploymentUpdateRequest.GreatestLowerBound(version)
	req := Request{
		Query:     reqQuery,
		Variables: variables,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.UpdateDeployment, nil
}

// GetDeployment - get a deployment
func (h ClientImplementation) GetDeployment(deploymentID string) (*Deployment, error) {
	reqQuery := DeploymentGetRequest.GreatestLowerBound(version)
	req := Request{
		Query:     reqQuery,
		Variables: map[string]interface{}{"id": deploymentID},
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return &res.Data.GetDeployment, nil
}

// UpdateDeploymentAirflow - update airflow on a deployment
func (h ClientImplementation) UpdateDeploymentAirflow(variables map[string]interface{}) (*Deployment, error) {
	req := Request{
		Query:     UpdateDeploymentAirflowRequest,
		Variables: variables,
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return res.Data.UpdateDeploymentAirflow, nil
}

// GetDeploymentConfig - get a deployment configuration
func (h ClientImplementation) GetDeploymentConfig(_ interface{}) (*DeploymentConfig, error) {
	dReq := Request{
		Query: DeploymentInfoRequest,
	}

	resp, err := dReq.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return &resp.Data.DeploymentConfig, nil
}

// ListDeploymentLogs - list logs from a deployment
func (h ClientImplementation) ListDeploymentLogs(filters ListDeploymentLogsRequest) ([]DeploymentLog, error) {
	req := Request{
		Query:     DeploymentLogsGetRequest,
		Variables: filters,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.DeploymentLog, nil
}

func (h ClientImplementation) UpdateDeploymentImage(updateReq UpdateDeploymentImageRequest) (interface{}, error) {
	req := Request{
		Query:     DeploymentImageUpdateRequest,
		Variables: updateReq,
	}

	_, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return nil, nil
}

func (h ClientImplementation) UpdateDeploymentRuntime(variables map[string]interface{}) (*Deployment, error) {
	req := Request{
		Query:     UpdateDeploymentRuntimeRequest,
		Variables: variables,
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return res.Data.UpdateDeploymentRuntime, nil
}

func (h ClientImplementation) CancelUpdateDeploymentRuntime(variables map[string]interface{}) (*Deployment, error) {
	req := Request{
		Query:     CancelUpdateDeploymentRuntimeRequest,
		Variables: variables,
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return res.Data.CancelUpdateDeploymentRuntime, nil
}
