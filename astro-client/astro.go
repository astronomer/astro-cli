package astro

import (
	"errors"
	"fmt"
)

type Client interface {
	GetUserInfo() (*Self, error)
	// Workspace
	ListWorkspaces(organizationID string) ([]Workspace, error)
	GetWorkspace(workspaceID string) (Workspace, error)
	// Deployment
	CreateDeployment(input *CreateDeploymentInput) (Deployment, error)
	UpdateDeployment(input *DeploymentUpdateInput) (Deployment, error)
	ListDeployments(input DeploymentsInput) ([]Deployment, error)
	DeleteDeployment(input DeploymentDeleteInput) (Deployment, error)
	GetDeploymentHistory(vars map[string]interface{}) (DeploymentHistory, error)
	GetDeploymentConfig() (DeploymentConfig, error)
	ModifyDeploymentVariable(input EnvironmentVariablesInput) ([]EnvironmentVariablesObject, error)
	InitiateDagDeployment(input InitiateDagDeploymentInput) (InitiateDagDeployment, error)
	ReportDagDeploymentStatus(input *ReportDagDeploymentStatusInput) (DagDeploymentStatus, error)
	// Image
	CreateImage(input ImageCreateInput) (*Image, error)
	DeployImage(input ImageDeployInput) (*Image, error)
	// Cluster
	ListClusters(organizationID string) ([]Cluster, error)
	// RuntimeRelease
	ListInternalRuntimeReleases() ([]RuntimeRelease, error)
	ListPublicRuntimeReleases() ([]RuntimeRelease, error)
	// UserInvite
	CreateUserInvite(input CreateUserInviteInput) (UserInvite, error)
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

func (c *HTTPClient) UpdateDeployment(input *DeploymentUpdateInput) (Deployment, error) {
	req := Request{
		Query:     DeploymentUpdate,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithClient(c)
	if err != nil {
		return Deployment{}, err
	}
	return resp.Data.DeploymentUpdate, nil
}

func (c *HTTPClient) ListDeployments(input DeploymentsInput) ([]Deployment, error) {
	req := Request{
		Query:     WorkspaceDeploymentsGetRequest,
		Variables: map[string]interface{}{"deploymentsInput": input},
	}

	resp, err := req.DoWithClient(c)
	if err != nil {
		return []Deployment{}, err
	}
	return resp.Data.GetDeployments, nil
}

func (c *HTTPClient) DeleteDeployment(input DeploymentDeleteInput) (Deployment, error) {
	req := Request{
		Query:     DeploymentDelete,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithClient(c)
	if err != nil {
		return Deployment{}, err
	}
	return resp.Data.DeploymentDelete, nil
}

func (c *HTTPClient) GetDeploymentHistory(vars map[string]interface{}) (DeploymentHistory, error) {
	req := Request{
		Query:     DeploymentHistoryQuery,
		Variables: vars,
	}

	resp, err := req.DoWithClient(c)
	if err != nil {
		return DeploymentHistory{}, err
	}
	return resp.Data.GetDeploymentHistory, nil
}

func (c *HTTPClient) GetDeploymentConfig() (DeploymentConfig, error) {
	req := Request{
		Query: GetDeploymentConfigOptions,
	}

	resp, err := req.DoWithClient(c)
	if err != nil {
		return DeploymentConfig{}, err
	}
	return resp.Data.GetDeploymentConfig, nil
}

func (c *HTTPClient) ModifyDeploymentVariable(input EnvironmentVariablesInput) ([]EnvironmentVariablesObject, error) {
	req := Request{
		Query:     DeploymentVariablesCreate,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithClient(c)
	if err != nil {
		return []EnvironmentVariablesObject{}, err
	}
	return resp.Data.DeploymentVariablesUpdate, nil
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

func (c *HTTPClient) CreateImage(input ImageCreateInput) (*Image, error) {
	req := Request{
		Query:     ImageCreate,
		Variables: map[string]interface{}{"imageCreateInput": input},
	}

	resp, err := req.DoWithClient(c)
	if err != nil {
		return nil, err
	}
	return resp.Data.CreateImage, nil
}

func (c *HTTPClient) DeployImage(input ImageDeployInput) (*Image, error) {
	req := Request{
		Query:     ImageDeploy,
		Variables: map[string]interface{}{"imageDeployInput": input},
	}

	resp, err := req.DoWithClient(c)
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

func (c *HTTPClient) ListInternalRuntimeReleases() ([]RuntimeRelease, error) {
	req := Request{
		Query:     InternalRuntimeReleases,
		Variables: map[string]interface{}{"channel": ""},
	}

	resp, err := req.DoWithClient(c)
	if err != nil {
		return []RuntimeRelease{}, err
	}
	return resp.Data.RuntimeReleases, nil
}

func (c *HTTPClient) ListPublicRuntimeReleases() ([]RuntimeRelease, error) {
	req := Request{
		Query:     PublicRuntimeReleases,
		Variables: map[string]interface{}{"channel": ""},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return []RuntimeRelease{}, err
	}
	return resp.Data.RuntimeReleases, nil
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
