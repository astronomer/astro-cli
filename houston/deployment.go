package houston

import "time"

// ListDeploymentsRequest - filters to list deployments according to set values
type ListDeploymentsRequest struct {
	WorkspaceID string `json:"workspaceId"`
	ReleaseName string `json:"releaseName"`
}

// ListDeploymentLogsRequest - filters to list logs from a deployment
type ListDeploymentLogsRequest struct {
	DeploymentID string    `json:"deploymentId"`
	Component    string    `json:"component"`
	Search       string    `json:"search"`
	Timestamp    time.Time `json:"timestamp"`
}

type UpdateDeploymentImageRequest struct {
	ReleaseName    string `json:"releaseName"`
	Image          string `json:"image"`
	AirflowVersion string `json:"airflowVersion"`
	RuntimeVersion string `json:"runtimeVersion"`
}

// CreateDeployment - create a deployment
func (h ClientImplementation) CreateDeployment(vars map[string]interface{}) (*Deployment, error) {
	req := Request{
		Query:     DeploymentCreateRequest,
		Variables: vars,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.CreateDeployment, nil
}

// DeleteDeployment - delete a deployment
func (h ClientImplementation) DeleteDeployment(deploymentID string, doHardDelete bool) (*Deployment, error) {
	req := Request{
		Query:     DeploymentDeleteRequest,
		Variables: map[string]interface{}{"deploymentId": deploymentID, "deploymentHardDelete": doHardDelete},
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

	req := Request{
		Query: DeploymentsGetRequest,
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

// UpdateDeployment - update a deployment
func (h ClientImplementation) UpdateDeployment(variables map[string]interface{}) (*Deployment, error) {
	req := Request{
		Query:     DeploymentUpdateRequest,
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
	req := Request{
		Query:     DeploymentGetRequest,
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
func (h ClientImplementation) GetDeploymentConfig() (*DeploymentConfig, error) {
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

func (h ClientImplementation) UpdateDeploymentImage(updateReq UpdateDeploymentImageRequest) error {
	req := Request{
		Query:     DeploymentImageUpdateRequest,
		Variables: updateReq,
	}

	_, err := req.DoWithClient(h.client)
	if err != nil {
		return handleAPIErr(err)
	}

	return nil
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
