package houston

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/logger"
)

var (
	HoustonClient ClientInterface

	HoustonConnectionErrMsg = "cannot connect to Astronomer. Try to log in with astro login or check your internet connection and user permissions.\n\nDetails: %w"

	errInaptPermissionsMsg       = "You do not have the appropriate permissions for that"
	errAuthTokenRefreshFailedMsg = "AUTH_TOKEN_REFRESH_FAILED" //nolint:gosec
	ErrVerboseInaptPermissions   = errors.New("you do not have the appropriate permissions for that: Your token has expired. Please log in again")
)

// ClientInterface - Interface that defines methods exposed by the Houston API
type ClientInterface interface {
	// user
	CreateUser(req CreateUserRequest) (*AuthUser, error)
	// workspace
	CreateWorkspace(req CreateWorkspaceRequest) (*Workspace, error)
	ListWorkspaces(interface{}) ([]Workspace, error)
	PaginatedListWorkspaces(req PaginatedListWorkspaceRequest) ([]Workspace, error)
	DeleteWorkspace(workspaceID string) (*Workspace, error)
	GetWorkspace(workspaceID string) (*Workspace, error)
	UpdateWorkspace(req UpdateWorkspaceRequest) (*Workspace, error)
	ValidateWorkspaceID(workspaceID string) (*Workspace, error)
	// workspace users and roles
	AddWorkspaceUser(req AddWorkspaceUserRequest) (*Workspace, error)
	DeleteWorkspaceUser(req DeleteWorkspaceUserRequest) (*Workspace, error)
	ListWorkspaceUserAndRoles(workspaceID string) ([]WorkspaceUserRoleBindings, error)
	ListWorkspacePaginatedUserAndRoles(req PaginatedWorkspaceUserRolesRequest) ([]WorkspaceUserRoleBindings, error)
	UpdateWorkspaceUserRole(req UpdateWorkspaceUserRoleRequest) (string, error)
	GetWorkspaceUserRole(req GetWorkspaceUserRoleRequest) (WorkspaceUserRoleBindings, error)
	// auth
	AuthenticateWithBasicAuth(req BasicAuthRequest) (string, error)
	GetAuthConfig(ctx *config.Context) (*AuthConfig, error)
	// deployment
	CreateDeployment(vars map[string]interface{}) (*Deployment, error)
	DeleteDeployment(req DeleteDeploymentRequest) (*Deployment, error)
	ListDeployments(filters ListDeploymentsRequest) ([]Deployment, error)
	UpdateDeployment(variables map[string]interface{}) (*Deployment, error)
	GetDeployment(deploymentID string) (*Deployment, error)
	UpdateDeploymentAirflow(variables map[string]interface{}) (*Deployment, error)
	UpdateDeploymentRuntime(variables map[string]interface{}) (*Deployment, error)
	CancelUpdateDeploymentRuntime(variables map[string]interface{}) (*Deployment, error)
	GetDeploymentConfig(interface{}) (*DeploymentConfig, error)
	ListDeploymentLogs(filters ListDeploymentLogsRequest) ([]DeploymentLog, error)
	UpdateDeploymentImage(req UpdateDeploymentImageRequest) (interface{}, error)
	// deployment users
	ListDeploymentUsers(filters ListDeploymentUsersRequest) ([]DeploymentUser, error)
	AddDeploymentUser(variables UpdateDeploymentUserRequest) (*RoleBinding, error)
	UpdateDeploymentUser(variables UpdateDeploymentUserRequest) (*RoleBinding, error)
	DeleteDeploymentUser(req DeleteDeploymentUserRequest) (*RoleBinding, error)
	// service account
	CreateDeploymentServiceAccount(variables *CreateServiceAccountRequest) (*DeploymentServiceAccount, error)
	DeleteDeploymentServiceAccount(req DeleteServiceAccountRequest) (*ServiceAccount, error)
	ListDeploymentServiceAccounts(deploymentID string) ([]ServiceAccount, error)
	CreateWorkspaceServiceAccount(variables *CreateServiceAccountRequest) (*WorkspaceServiceAccount, error)
	DeleteWorkspaceServiceAccount(req DeleteServiceAccountRequest) (*ServiceAccount, error)
	ListWorkspaceServiceAccounts(workspaceID string) ([]ServiceAccount, error)
	// app
	GetAppConfig(interface{}) (*AppConfig, error)
	GetAvailableNamespaces(interface{}) ([]Namespace, error)
	GetPlatformVersion(interface{}) (string, error)
	// runtime
	GetRuntimeReleases(airflowVersion string) (RuntimeReleases, error)
	// teams
	GetTeam(teamID string) (*Team, error)
	GetTeamUsers(teamID string) ([]User, error)
	ListTeams(req ListTeamsRequest) (ListTeamsResp, error)
	CreateTeamSystemRoleBinding(req SystemRoleBindingRequest) (string, error)
	DeleteTeamSystemRoleBinding(req SystemRoleBindingRequest) (string, error)
	// deployment teams
	AddDeploymentTeam(req AddDeploymentTeamRequest) (*RoleBinding, error)
	RemoveDeploymentTeam(req RemoveDeploymentTeamRequest) (*RoleBinding, error)
	ListDeploymentTeamsAndRoles(deploymentID string) ([]Team, error)
	UpdateDeploymentTeamRole(req UpdateDeploymentTeamRequest) (*RoleBinding, error)
	// workspace teams and roles
	AddWorkspaceTeam(req AddWorkspaceTeamRequest) (*Workspace, error)
	DeleteWorkspaceTeam(req DeleteWorkspaceTeamRequest) (*Workspace, error)
	ListWorkspaceTeamsAndRoles(workspaceID string) ([]Team, error)
	UpdateWorkspaceTeamRole(req UpdateWorkspaceTeamRoleRequest) (string, error)
	GetWorkspaceTeamRole(req GetWorkspaceTeamRoleRequest) (*Team, error)
	ListPaginatedDeployments(req PaginatedDeploymentsRequest) ([]Deployment, error)
}

// ClientImplementation - implementation of the Houston Client Interface
type ClientImplementation struct {
	client *Client
}

// NewClient - initialized the Houston Client object with proper HTTP Client configuration
// set as a variable so we can change it to return mock houston clients in tests
var NewClient = func(c *httputil.HTTPClient) ClientInterface {
	client := newInternalClient(c)
	return &ClientImplementation{
		client: client,
	}
}

// Client containers the logger and HTTPClient used to communicate with the HoustonAPI
type Client struct {
	HTTPClient *httputil.HTTPClient
}

func NewHTTPClient() *httputil.HTTPClient {
	httpClient := httputil.NewHTTPClient()
	// configure http transport
	dialTimeout := config.CFG.HoustonDialTimeout.GetInt()
	// #nosec
	httpClient.HTTPClient.Transport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: time.Duration(dialTimeout) * time.Second,
		}).Dial,
		TLSHandshakeTimeout: time.Duration(dialTimeout) * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: config.CFG.HoustonSkipVerifyTLS.GetBool()},
	}
	return httpClient
}

// newInternalClient returns a new Client with the logger and HTTP Client setup.
func newInternalClient(c *httputil.HTTPClient) *Client {
	return &Client{
		HTTPClient: c,
	}
}

type Request struct {
	Query     string      `json:"query"`
	Variables interface{} `json:"variables"`
}

// Do (request) is a wrapper to more easily pass variables to a Client.Do request
func (r *Request) DoWithClient(api *Client) (*Response, error) {
	req, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	doOpts := &httputil.DoOptions{
		Data: req,
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	return api.Do(doOpts)
}

// Do (request) is a wrapper to more easily pass variables to a Client.Do request
func (r *Request) Do() (*Response, error) {
	return r.DoWithClient(newInternalClient(httputil.NewHTTPClient()))
}

// Do fetches the current context, and returns Houston API response, error
func (c *Client) Do(doOpts *httputil.DoOptions) (*Response, error) {
	cl, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	return c.DoWithContext(doOpts, &cl)
}

// DoWithContext executes a query against the Houston API, logging out any errors contained in the response object
func (c *Client) DoWithContext(doOpts *httputil.DoOptions, ctx *config.Context) (*Response, error) {
	// set headers
	if ctx.Token != "" {
		doOpts.Headers["authorization"] = ctx.Token
	}
	logger.Logger.Debugf("Request Data: %v\n", string(doOpts.Data))
	doOpts.Method = http.MethodPost
	doOpts.Path = ctx.GetSoftwareAPIURL()
	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do(doOpts)
	if err != nil {
		logger.Logger.Debugf("HTTP request ERROR: %s", err.Error())
		return nil, err
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}

	response = httputil.HTTPResponse{
		Raw:  httpResponse,
		Body: string(body),
	}
	decode := Response{}
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&decode)
	if err != nil {
		return nil, fmt.Errorf("failed to JSON decode Houston response: %w", err)
	}
	logger.Logger.Debugf("Response Data: %v\n", string(body))
	// Houston Specific Errors
	if decode.Errors != nil {
		errMsg := decode.Errors[0].Message
		if errMsg == errInaptPermissionsMsg || errMsg == errAuthTokenRefreshFailedMsg {
			return nil, ErrVerboseInaptPermissions
		}
		return nil, fmt.Errorf("%s", decode.Errors[0].Message)
	}

	return &decode, nil
}
