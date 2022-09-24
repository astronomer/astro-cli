package astro

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

type Client interface {
	GetUserInfo() (*Self, error)
	// Workspace
	ListWorkspaces(organizationID string) ([]Workspace, error)
	GetWorkspace(workspaceID string) (Workspace, error)
	// Deployment
	CreateDeployment(input *CreateDeploymentInput) (Deployment, error)
	UpdateDeployment(input *UpdateDeploymentInput) (Deployment, error)
	ListDeployments(organizationID, workspaceID string) ([]Deployment, error)
	GetDeployment(deploymentID string) (Deployment, error)
	DeleteDeployment(input DeleteDeploymentInput) (Deployment, error)
	GetDeploymentHistory(vars map[string]interface{}) (DeploymentHistory, error)
	GetDeploymentConfig() (DeploymentConfig, error)
	ModifyDeploymentVariable(input EnvironmentVariablesInput) ([]EnvironmentVariablesObject, error)
	InitiateDagDeployment(input InitiateDagDeploymentInput) (InitiateDagDeployment, error)
	ReportDagDeploymentStatus(input *ReportDagDeploymentStatusInput) (DagDeploymentStatus, error)
	// Image
	CreateImage(input CreateImageInput) (*Image, error)
	DeployImage(input DeployImageInput) (*Image, error)
	// Cluster
	ListClusters(organizationID string) ([]Cluster, error)
	// UserInvite
	CreateUserInvite(input CreateUserInviteInput) (UserInvite, error)
	// WorkerQueues
	GetWorkerQueueOptions() (WorkerQueueDefaultOptions, error)
	// Organizations
	GetOrganizations() ([]Organization, error)
	GetOrganizationAuditLogs() (string, error)
}

func (c *HTTPClient) GetUserInfo() (*Self, error) {
	req := Request{
		Query: SelfQuery,
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return nil, err
	}

	if resp.Data.SelfQuery == nil {
		fmt.Printf("Something went wrong! Try again or contact Astronomer Support")
		return nil, errors.New("something went wrong! Try again or contact Astronomer Support") //nolint:goerr113
	}

	return resp.Data.SelfQuery, nil
}

func (c *HTTPClient) ListWorkspaces(organizationID string) ([]Workspace, error) {
	wsReq := Request{
		Query:     WorkspacesGetRequest,
		Variables: map[string]interface{}{"organizationId": organizationID},
	}

	wsResp, err := wsReq.DoWithPublicClient(c)
	if err != nil {
		return []Workspace{}, err
	}
	return wsResp.Data.GetWorkspaces, nil
}

func (c *HTTPClient) CreateDeployment(input *CreateDeploymentInput) (Deployment, error) {
	req := Request{
		Query:     CreateDeployment,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return Deployment{}, err
	}
	return resp.Data.CreateDeployment, nil
}

func (c *HTTPClient) UpdateDeployment(input *UpdateDeploymentInput) (Deployment, error) {
	req := Request{
		Query:     UpdateDeployment,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return Deployment{}, err
	}
	return resp.Data.UpdateDeployment, nil
}

func (c *HTTPClient) ListDeployments(organizationID, workspaceID string) ([]Deployment, error) {
	req := Request{
		Query:     WorkspaceDeploymentsGetRequest,
		Variables: map[string]interface{}{"organizationId": organizationID, "workspaceId": workspaceID},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return []Deployment{}, err
	}
	return resp.Data.GetDeployments, nil
}

func (c *HTTPClient) GetDeployment(deploymentID string) (Deployment, error) {
	req := Request{
		Query:     GetDeployment,
		Variables: map[string]interface{}{"deploymentId": deploymentID},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return Deployment{}, err
	}
	return resp.Data.GetDeployment, nil
}

func (c *HTTPClient) DeleteDeployment(input DeleteDeploymentInput) (Deployment, error) {
	req := Request{
		Query:     DeleteDeployment,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return Deployment{}, err
	}
	return resp.Data.DeleteDeployment, nil
}

func (c *HTTPClient) GetDeploymentHistory(vars map[string]interface{}) (DeploymentHistory, error) {
	req := Request{
		Query:     DeploymentHistoryQuery,
		Variables: vars,
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return DeploymentHistory{}, err
	}
	return resp.Data.GetDeploymentHistory, nil
}

func (c *HTTPClient) GetDeploymentConfig() (DeploymentConfig, error) {
	req := Request{
		Query: GetDeploymentConfigOptions,
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return DeploymentConfig{}, err
	}
	return resp.Data.GetDeploymentConfig, nil
}

func (c *HTTPClient) ModifyDeploymentVariable(input EnvironmentVariablesInput) ([]EnvironmentVariablesObject, error) {
	req := Request{
		Query:     CreateDeploymentVariables,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return []EnvironmentVariablesObject{}, err
	}
	return resp.Data.UpdateDeploymentVariables, nil
}

func (c *HTTPClient) InitiateDagDeployment(input InitiateDagDeploymentInput) (InitiateDagDeployment, error) {
	req := Request{
		Query:     DagDeploymentInitiate,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return InitiateDagDeployment{}, err
	}
	return resp.Data.InitiateDagDeployment, nil
}

func (c *HTTPClient) ReportDagDeploymentStatus(input *ReportDagDeploymentStatusInput) (DagDeploymentStatus, error) {
	req := Request{
		Query:     ReportDagDeploymentStatus,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return DagDeploymentStatus{}, err
	}
	return resp.Data.ReportDagDeploymentStatus, nil
}

func (c *HTTPClient) CreateImage(input CreateImageInput) (*Image, error) {
	req := Request{
		Query:     CreateImage,
		Variables: map[string]interface{}{"imageCreateInput": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return nil, err
	}
	return resp.Data.CreateImage, nil
}

func (c *HTTPClient) DeployImage(input DeployImageInput) (*Image, error) {
	req := Request{
		Query:     DeployImage,
		Variables: map[string]interface{}{"imageDeployInput": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return nil, err
	}
	return resp.Data.DeployImage, nil
}

func (c *HTTPClient) ListClusters(organizationID string) ([]Cluster, error) {
	req := Request{
		Query:     GetClusters,
		Variables: map[string]interface{}{"organizationId": organizationID},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return []Cluster{}, err
	}
	return resp.Data.GetClusters, nil
}

// CreateUserInvite create a user invite request
func (c *HTTPClient) CreateUserInvite(input CreateUserInviteInput) (UserInvite, error) {
	req := Request{
		Query:     CreateUserInvite,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return UserInvite{}, err
	}
	return resp.Data.CreateUserInvite, nil
}

// GetWorkspace returns information about the workspace
func (c *HTTPClient) GetWorkspace(workspaceID string) (Workspace, error) {
	wsReq := Request{
		Query:     GetWorkspace,
		Variables: map[string]interface{}{"workspaceId": workspaceID},
	}

	wsResp, err := wsReq.DoWithPublicClient(c)
	if err != nil {
		return Workspace{}, err
	}
	return wsResp.Data.GetWorkspace, nil
}

// GetWorkerQueueOptions gets the worker-queue default options
func (c *HTTPClient) GetWorkerQueueOptions() (WorkerQueueDefaultOptions, error) {
	wqReq := Request{
		Query: GetWorkerQueueOptions,
	}

	wqResp, err := wqReq.DoWithPublicClient(c)
	if err != nil {
		return WorkerQueueDefaultOptions{}, err
	}
	return wqResp.Data.GetWorkerQueueOptions, nil
}

// Gets a list of Organizations
func (c *HTTPClient) GetOrganizations() ([]Organization, error) {
	req := Request{
		Query: GetOrganizations,
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return []Organization{}, err
	}
	return resp.Data.GetOrganizations, nil
}

func (c *HTTPClient) GetOrganizationAuditLogs() (string, error) {
	doOpts := httputil.DoOptions{
		Method:  http.MethodGet,
		Headers: make(map[string]string),
		Path:    "/organizations/astronomer/audit-logs",
	}
	resp, err := c.DoPublicRESTQuery(doOpts)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}
