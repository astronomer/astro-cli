package astro

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

var organizationShortNameRegex = regexp.MustCompile("[^a-z0-9-]")

type Client interface {
	// Deployment
	CreateDeployment(input *CreateDeploymentInput) (Deployment, error)
	UpdateDeployment(input *UpdateDeploymentInput) (Deployment, error)
	ListDeployments(organizationID, workspaceID string) ([]Deployment, error)
	GetDeployment(deploymentID string) (Deployment, error)
	DeleteDeployment(input DeleteDeploymentInput) (Deployment, error)
	GetDeploymentHistory(vars map[string]interface{}) (DeploymentHistory, error)
	GetDeploymentConfig() (DeploymentConfig, error)
	GetDeploymentConfigWithOrganization(organizationID string) (DeploymentConfig, error)
	ModifyDeploymentVariable(input EnvironmentVariablesInput) ([]EnvironmentVariablesObject, error)
	InitiateDagDeployment(input InitiateDagDeploymentInput) (InitiateDagDeployment, error)
	ReportDagDeploymentStatus(input *ReportDagDeploymentStatusInput) (DagDeploymentStatus, error)
	// Image
	CreateImage(input CreateImageInput) (*Image, error)
	DeployImage(input *DeployImageInput) (*Image, error)
	// WorkerQueues
	GetWorkerQueueOptions() (WorkerQueueDefaultOptions, error)
	// Organizations
	GetOrganizationAuditLogs(orgName string, earliest int) (io.ReadCloser, error)
	// Alert Emails
	UpdateAlertEmails(input UpdateDeploymentAlertsInput) (DeploymentAlerts, error)
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

func (c *HTTPClient) GetDeploymentConfigWithOrganization(organizationID string) (DeploymentConfig, error) {
	req := Request{
		Query:     GetDeploymentConfigOptionsWithOrganization,
		Variables: map[string]interface{}{"organizationId": organizationID},
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

func (c *HTTPClient) DeployImage(input *DeployImageInput) (*Image, error) {
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

func (c *HTTPClient) GetOrganizationAuditLogs(orgName string, earliest int) (io.ReadCloser, error) {
	// An organization short name has only lower case characters and is stripped of non-alphanumeric characters (expect for hyphens)
	orgShortName := strings.ToLower(orgName)
	orgShortName = organizationShortNameRegex.ReplaceAllString(orgShortName, "")
	doOpts := &httputil.DoOptions{
		Method:  http.MethodGet,
		Headers: make(map[string]string),
		Path:    fmt.Sprintf("/organizations/%s/audit-logs?earliest=%d", orgShortName, earliest),
	}
	streamBuffer, err := c.DoPublicRESTStreamQuery(doOpts)
	if err != nil {
		return nil, err
	}
	return streamBuffer, nil
}

// UpdateAlertEmails updates alert emails for a deployment
func (c *HTTPClient) UpdateAlertEmails(input UpdateDeploymentAlertsInput) (DeploymentAlerts, error) {
	req := Request{
		Query:     UpdateDeploymentAlerts,
		Variables: map[string]interface{}{"input": input},
	}

	resp, err := req.DoWithPublicClient(c)
	if err != nil {
		return DeploymentAlerts{}, err
	}
	return resp.Data.DeploymentAlerts, nil
}
