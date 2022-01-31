package houston

import "time"

// ListDeploymentsRequest - filters to list deployments according to set values
type ListDeploymentsRequest struct {
	WorkspaceID string `json:"workspaceId"`
	ReleaseName string `json:"releaseName"`
}

// ListDeploymentLogsRequest - filters to list logs from a deployment
type ListDeploymentLogsRequest struct {
	DeploymentID string `json:"deploymentId"`
	Component    string `json:"component"`
	Search       string `json:"search"`
	Timestamp    time.Time `json:"timestamp"`
}

// CreateDeployment - create a deployment
func (h HoustonClientImplementation) CreateDeployment(vars map[string]interface{}) (*Deployment, error) {
	req := Request{
		Query:     DeploymentCreateRequest,
		Variables: vars,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return r.Data.CreateDeployment, nil
}

// DeleteDeployment - delete a deployment
func (h HoustonClientImplementation) DeleteDeployment(deploymentID string, doHardDelete bool) (*Deployment, error) {
	req := Request{
		Query:     DeploymentDeleteRequest,
		Variables: map[string]interface{}{"deploymentId": deploymentID, "deploymentHardDelete": doHardDelete},
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return res.Data.DeleteDeployment, nil
}

// ListDeployments - List deployments from the API
func (h HoustonClientImplementation) ListDeployments(filters ListDeploymentsRequest) ([]Deployment, error) {
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
		return nil, err
	}

	return res.Data.GetDeployments, nil
}

// UpdateDeployment - update a deployment
func (h HoustonClientImplementation) UpdateDeployment(variables map[string]interface{}) (*Deployment, error) {
	req := Request{
		Query:     DeploymentUpdateRequest,
		Variables: variables,
	}
	
	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return r.Data.UpdateDeployment, nil
}

// GetDeployment - get a deployment
func (h HoustonClientImplementation) GetDeployment(deploymentID string) (*Deployment, error) {
	req := Request{
		Query:     DeploymentGetRequest,
		Variables: map[string]interface{}{"id": deploymentID},
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return &res.Data.GetDeployment, nil
}

// UpdateDeploymentAirflow - update airflow on a deployment
func (h HoustonClientImplementation) UpdateDeploymentAirflow(variables map[string]interface{}) (*Deployment, error) {
	req := Request{
		Query:     UpdateDeploymentAirflowRequest,
		Variables: variables,
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return res.Data.UpdateDeploymentAirflow, nil
}

// GetDeploymentConfig - get a deployment configuration
func (h HoustonClientImplementation) GetDeploymentConfig() (*DeploymentConfig, error) {
	dReq := Request{
		Query: DeploymentInfoRequest,
	}

	resp, err := dReq.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}
	
	return &resp.Data.DeploymentConfig, nil
}

// ListDeploymentLogs - list logs from a deployment
func (h HoustonClientImplementation) ListDeploymentLogs(filters ListDeploymentLogsRequest) ([]DeploymentLog, error) {
	req := Request{
		Query: DeploymentLogsGetRequest,
		Variables: filters,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}
	
	return r.Data.DeploymentLog, nil
}
