package houston

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/pkg/httputil"

	newLogger "github.com/sirupsen/logrus"
)

var (
	ErrInaptPermissions        = errors.New("You do not have the appropriate permissions for that") //nolint
	ErrVerboseInaptPermissions = errors.New("you do not have the appropriate permissions for that: Your token has expired. Please log in again")
)

// HoustonClientInterface - Interface that defines methods exposed by the Houston API
type HoustonClientInterface interface {
	// user
	CreateUser(email, password string) (*AuthUser, error)
	// workspace
	CreateWorkspace(label, description string) (*Workspace, error)
	ListWorkspaces() ([]Workspace, error)
	DeleteWorkspace(workspaceID string) (*Workspace, error)
	GetWorkspace(workspaceID string) (*Workspace, error)
	UpdateWorkspace(workspaceID string, args map[string]string) (*Workspace, error)
	// workspace users and roles
	AddUserToWorkspace(workspaceID, email, role string) (*Workspace, error)
	DeleteUserFromWorkspace(workspaceID, userID string) (*Workspace, error)
	ListUserAndRolesFromWorkspace(workspaceID string) (*Workspace, error)
	UpdateUserRoleInWorkspace(workspaceID, email, role string) (string, error)
	GetUserRoleInWorkspace(workspaceID, email string) (WorkspaceUserRoleBindings, error)
	// auth
	AuthenticateWithBasicAuth(username, password string) (string, error)
	GetAuthConfig() (*AuthConfig, error)
	// deployment
	CreateDeployment(vars map[string]interface{}) (*Deployment, error)
	DeleteDeployment(deploymentID string, doHardDelete bool) (*Deployment, error)
	ListDeployments(filters ListDeploymentsRequest) ([]Deployment, error)
	UpdateDeployment(variables map[string]interface{}) (*Deployment, error)
	GetDeployment(deploymentID string) (*Deployment, error)
	UpdateDeploymentAirflow(variables map[string]interface{}) (*Deployment, error)
	GetDeploymentConfig() (*DeploymentConfig, error)
	ListDeploymentLogs(filters ListDeploymentLogsRequest) ([]DeploymentLog, error)
	// deployment users
	ListUsersInDeployment(filters ListUsersInDeploymentRequest) ([]DeploymentUser, error)
	AddUserToDeployment(variables UpdateUserInDeploymentRequest) (*RoleBinding, error)
	UpdateUserInDeployment(variables UpdateUserInDeploymentRequest) (*RoleBinding, error)
	DeleteUserFromDeployment(deploymentID, email string) (*RoleBinding, error)
	// service account
	CreateServiceAccountInDeployment(variables CreateServiceAccountRequest) (*DeploymentServiceAccount, error)
	DeleteServiceAccountFromDeployment(deploymentID, serviceAccountID string) (*ServiceAccount, error)
	ListServiceAccountsInDeployment(deploymentID string) ([]ServiceAccount, error)
	CreateServiceAccountInWorkspace(variables CreateServiceAccountRequest) (*WorkspaceServiceAccount, error)
	DeleteServiceAccountFromWorkspace(workspaceID, serviceAccountID string) (*ServiceAccount, error)
	ListServiceAccountsInWorkspace(workspaceID string) ([]ServiceAccount, error)
	// app
	GetAppConfig() (*AppConfig, error)
	GetAvailableNamespaces() ([]Namespace, error)
}

// HoustonClientImplementation - implementation of the Houston Client Interface
type HoustonClientImplementation struct {
	Client *Client
}

// TODO: RENAME THIS METHOD ONCE TESTS ARE REFACTORED TOO
// Init - initialized the Houston Client singleton object with proper HTTP Client configuration
// set as a variable so we can change it to return mock houston clients
var Init = func (c *httputil.HTTPClient) HoustonClientInterface {
	client := NewHoustonClient(c)
	return &HoustonClientImplementation{
		Client: client,
	}
}

// Client containers the logger and HTTPClient used to communicate with the HoustonAPI
type Client struct {
	HTTPClient *httputil.HTTPClient
}

// NewHoustonClient returns a new Client with the logger and HTTP Client setup.
func NewHoustonClient(c *httputil.HTTPClient) *Client {
	return &Client{
		HTTPClient: c,
	}
}

// GraphQLQuery wraps a graphql query string
type GraphQLQuery struct {
	Query string `json:"query"`
}

type Request struct {
	Query     string                 `json:"query"`
	Variables interface{} `json:"variables"`
}

// Do (request) is a wrapper to more easily pass variables to a Client.Do request
func (r *Request) DoWithClient(api *Client) (*Response, error) {
	doOpts := httputil.DoOptions{
		Data: r,
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	return api.Do(doOpts)
}

// Do (request) is a wrapper to more easily pass variables to a Client.Do request
func (r *Request) Do() (*Response, error) {
	return r.DoWithClient(NewHoustonClient(httputil.NewHTTPClient()))
}

// Do executes a query against the Houston API, logging out any errors contained in the response object
func (c *Client) Do(doOpts httputil.DoOptions) (*Response, error) {
	cl, err := cluster.GetCurrentCluster()
	if err != nil {
		return nil, err
	}

	// set headers
	if cl.Token != "" {
		doOpts.Headers["authorization"] = cl.Token
	}
	newLogger.Debugf("Request Data: %v\n", doOpts.Data)
	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do("POST", cl.GetAPIURL(), &doOpts)
	if err != nil {
		newLogger.Debugf("HTTP request ERROR: %s", err.Error())
		return nil, err
	}
	defer httpResponse.Body.Close()

	body, err := ioutil.ReadAll(httpResponse.Body)
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
	newLogger.Debugf("Response Data: %v\n", string(body))
	// Houston Specific Errors
	if decode.Errors != nil {
		err = fmt.Errorf("%s", decode.Errors[0].Message) //nolint:goerr113
		if err.Error() == ErrInaptPermissions.Error() {
			return nil, ErrVerboseInaptPermissions
		}
		return nil, err
	}

	return &decode, nil
}
