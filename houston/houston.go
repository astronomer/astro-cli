package houston

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
	// "github.com/sirupsen/logrus"

	"github.com/astronomerio/astro-cli/cluster"
	"github.com/astronomerio/astro-cli/pkg/httputil"
)

var (
	authConfigGetRequest = `
	query GetAuthConfig {
		authConfig(redirect: "") {
		  localEnabled
		  googleEnabled
		  githubEnabled
		  auth0Enabled
		  googleOAuthUrl
		}
	  }`

	deploymentCreateRequest = `
	mutation CreateDeployment {
		createDeployment(	
			label: "%s",
			type: "airflow",
			workspaceUuid: "%s"
		) {	
			uuid
			type
			label
			releaseName
			version
			createdAt
			updatedAt
		}
	}`

	deploymentDeleteRequest = `
	mutation DeleteDeployment {
		deleteDeployment(deploymentUuid: "%s") {
			uuid
			type
			label
			releaseName
			version
			createdAt
			updatedAt
		}
	}`

	deploymentGetRequest = `
	query GetDeployment {
	  deployments(
			deploymentUuid: "%s"
		) {	
			uuid
			type
			label
			releaseName
			version
			createdAt
			updatedAt
	  }
	}`

	deploymentsGetRequest = `
	query GetDeployments {
	  deployments(workspaceUuid: "%s") {
		uuid
		type
		label
		releaseName
		workspace {
          uuid
		}
		deployInfo {
			latest
			next
		}
		version
		createdAt
		updatedAt
	  }
	}`

	deploymentsGetAllRequest = `
	query GetAllDeployments {
	  deployments {
		uuid
		type
		label
		releaseName
		workspace {
			uuid
		}
		deployInfo {
			latest
			next
		}
		version
		createdAt
		updatedAt
	  }
	}`

	deploymentUpdateRequest = `
	mutation UpdateDeplomyent {
		updateDeployment(deploymentUuid:"%s",
		  payload: %s
		) {
		  uuid
		  type
		  label
		  releaseName
		  version
		}
	  }`

	serviceAccountCreateRequest = `
	mutation CreateServiceAccount {
		createServiceAccount(
			label: "%s",
			category: "%s",
			entityType: %s
		) {
		  apiKey
		  label
		  category
		  entityType
		  active
		}
	  }`

	serviceAccountDeleteRequest = `
	mutation DeleteServiceAccount {
		deleteServiceAccount(
		serviceAccountUuid:"%s"
		) {
		  uuid
		  label
		  category
		  entityType
		  entityUuid
		}
	  }`

	serviceAccountsGetRequest = `
	query GetServiceAccount {
		serviceAccounts(
			entityType:%s,
      entityUuid:"%s"
		) {
      uuid
      apiKey
      label
      category
      entityType
      entityUuid
      active
      createdAt
      updatedAt
      lastUsedAt
		}
	  }`

	tokenBasicCreateRequest = `
	mutation createBasicToken {
	  createToken(
		  identity:"%s",
		  password:"%s"
		) {
			user {
				uuid
				username
				status
				createdAt
				updatedAt
			}
			token {
				value
			}
		}
	}`

	userCreateRequest = `
	mutation CreateUser {
		createUser(
			email: "%s",
			password: "%s"
		) {
			user {
				uuid
				username
				status
				createdAt
				updatedAt
			}
			token {
				value
			}
		}
	}`

	userGetAllRequest = `
	query GetUsers {
		users {
			uuid
			emails {
				address
				verified
				primary
				createdAt
				updatedAt
			}
			fullName
			username
			status
			createdAt
			updatedAt
		}
	}`

	workspaceAllGetRequest = `
	query GetWorkspaces {
		workspaces {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	workspaceGetRequest = `
	query GetWorkspaces {
		workspaces(workspaceUuid:"%s") {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	workspaceCreateRequest = `
	mutation CreateWorkspace {
		createWorkspace(
			label: "%s",
			description: "%s"
		) {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	workspaceDeleteRequest = `
	mutation DeleteWorkspace {
		deleteWorkspace(workspaceUuid: "%s") {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	workspaceUpdateRequest = `
	mutation UpdateWorkspace {
		updateWorkspace(workspaceUuid:"%s",
		  payload: %s
		) {
		  uuid
		  description
		  label
		  active
		}
	  }`

	workspaceUserAddRequest = `
	mutation AddWorkspaceUser {
		workspaceAddUser(
			workspaceUuid: "%s",
			email: "%s",
		) {
			uuid
			label
			description
			active
			users {
				uuid
				username
			}
			createdAt
			updatedAt
		}
	}`

	workspaceUserRemoveRequest = `
	mutation RemoveWorkspaceUser {
		workspaceRemoveUser(
			workspaceUuid: "%s",
			userUuid: "%s",
		) {
			uuid
			label
			description
			active
			users {
				uuid
				username
			}
			createdAt
			updatedAt
		}
	}`
)

// Client containers the logger and HTTPClient used to communicate with the HoustonAPI
type Client struct {
	HTTPClient *httputil.HTTPClient
}

// NewHoustonClient returns a new Client with the logger and HTTP client setup.
func NewHoustonClient(c *httputil.HTTPClient) *Client {
	return &Client{
		HTTPClient: c,
	}
}

// GraphQLQuery wraps a graphql query string
type GraphQLQuery struct {
	Query string `json:"query"`
}

// QueryHouston executes a query against the Houston API, logging out any errors contained in the response object
func (c *Client) QueryHouston(query string) (*HoustonResponse, error) {
	doOpts := httputil.DoOptions{
		Data: GraphQLQuery{query},
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	cl, err := cluster.GetCurrentCluster()
	if err != nil {
		return nil, err
	}

	// set headers
	if cl.Token != "" {
		doOpts.Headers["authorization"] = cl.Token
	}

	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do("POST", cl.GetAPIURL(), &doOpts)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()

	// strings.NewReader(jsonStream)
	body, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}

	response = httputil.HTTPResponse{
		Raw:  httpResponse,
		Body: string(body),
	}
	decode := HoustonResponse{}
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&decode)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to JSON decode Houston response")
	}

	// Houston Specific Errors
	if decode.Errors != nil {
		return nil, errors.New(decode.Errors[0].Message)
	}

	return &decode, nil
}

// AddWorkspaceUser sends request to Houston to add a user to a workspace
// Returns a Workspace object
func (c *Client) AddWorkspaceUser(workspaceId, email string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceUserAddRequest, workspaceId, email)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "AddWorkspaceUser Failed")
	}

	return response.Data.AddWorkspaceUser, nil
}

// CreateDeployment sends request to Houston to create a new Airflow deployment cluster
// Returns a Deployment object
func (c *Client) CreateDeployment(label, wsId string) (*Deployment, error) {
	request := fmt.Sprintf(deploymentCreateRequest, label, wsId)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateDeployment Failed")
	}

	return response.Data.CreateDeployment, nil
}

// CreateBasicToken sends request to Houston in order to fetch a Basic user Token
// Returns an AuthUser object
func (c *Client) CreateBasicToken(email, password string) (*AuthUser, error) {
	request := fmt.Sprintf(tokenBasicCreateRequest, email, password)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateBasicToken Failed")
	}

	return response.Data.CreateToken, nil
}

// CreateServiceAccount sends a request to Houston in order to fetch a newly created service account
// Returns a ServiceAccount object
func (c *Client) CreateServiceAccount(uuid, label, category, entityType string) (*ServiceAccount, error) {
	request := fmt.Sprintf(serviceAccountCreateRequest, label, category, entityType)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateServiceAccount Failed")
	}

	return response.Data.CreateServiceAccount, nil
}

// CreateUser sends request to request to Houston in order to create a new platform User
// Returns an AuthUser object containing an token
func (c *Client) CreateUser(email string, password string) (*AuthUser, error) {
	request := fmt.Sprintf(userCreateRequest, email, password)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateUser Failed")
	}
	return response.Data.CreateUser, nil
}

// CreateWorkspace will send a request to Houston to create a new workspace
// Returns an object representing created workspace
func (c *Client) CreateWorkspace(label, description string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceCreateRequest, label, description)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateWorkspace Failed")
	}

	return response.Data.CreateWorkspace, nil
}

// DeleteDeployment sends a request to Houston in order to delete a specified deployment
// Returns a Deployment object which represents the Deployment that was deleted
func (c *Client) DeleteDeployment(uuid string) (*Deployment, error) {
	request := fmt.Sprintf(deploymentDeleteRequest, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "DeleteDeployment Failed")
	}

	return response.Data.DeleteDeployment, nil
}

// DeleteWorkspace will send a request to Houston to delete a workspace
// Returns an object representing deleted workspace
func (c *Client) DeleteWorkspace(uuid string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceDeleteRequest, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "DeleteWorkspace Failed")
	}

	return response.Data.DeleteWorkspace, nil
}

// GetAllDeployments will request all airflow deployments from Houston
// Returns a []Deployment structure with deployment details
func (c *Client) GetAllDeployments() ([]Deployment, error) {
	request := deploymentsGetAllRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetAllDeployments Failed")
	}

	return response.Data.GetDeployments, nil
}

// DeleteServiceAccount will send a request to Houston to delete a service account
// Returns an object representing deleted service account
func (c *Client) DeleteServiceAccount(uuid string) (*ServiceAccount, error) {
	request := fmt.Sprintf(serviceAccountDeleteRequest, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "DeleteServiceAccount Request")
	}

	return response.Data.DeleteServiceAccount, nil
}

// GetDeployments will request all airflow deployments from Houston
// Returns a []Deployment structure with deployment details
func (c *Client) GetDeployments(ws string) ([]Deployment, error) {
	request := fmt.Sprintf(deploymentsGetRequest, ws)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetDeployments Failed")
	}

	return response.Data.GetDeployments, nil
}

// GetDeployment will request a specific airflow deployments from Houston by uuid
// Returns a Deployment structure with deployment details
// func (c *Client) GetDeployment(deploymentUuid string) (*Deployment, error) {
// 	request := fmt.Sprintf(deploymentGetRequest, deploymentUuid)

// 	response, err := c.QueryHouston(request)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "GetDeployment Failed")
// 	}

// 	if len(response.Data.GetDeployments) == 0 {
// 		return nil, fmt.Errorf("deployment not found for uuid \"%s\"", deploymentUuid)
// 	}
// 	return &response.Data.GetDeployments[0], nil
// }

// GetAuthConfig will get authentication configuration from houston
// Returns the requested AuthConfig object
func (c *Client) GetAuthConfig() (*AuthConfig, error) {
	request := authConfigGetRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetAuthConfig Failed")
	}

	return response.Data.GetAuthConfig, nil
}

// GetServiceAccounts will get GetServiceAccounts from houston
// Returns slice of GetServiceAccounts
func (c *Client) GetServiceAccounts(entityType, uuid string) ([]ServiceAccount, error) {
	request := fmt.Sprintf(serviceAccountsGetRequest, entityType, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetServiceAccounts Failed")
	}

	return response.Data.GetServiceAccounts, nil
}

// GetWorkspace returns all available workspaces from houston API
// Returns a workspace struct
func (c *Client) GetWorkspace(uuid string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceGetRequest, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetWorkspace Failed")
	}

	return response.Data.GetWorkspace, nil
}

// GetWorkspaceAll returns all available workspaces from houston API
// Returns a slice of all Workspaces a user has access to
func (c *Client) GetWorkspaceAll() ([]Workspace, error) {
	request := workspaceAllGetRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetWorkspaceAll Failed")
	}

	return response.Data.GetWorkspaces, nil
}

// GetUserAll sends a request to Houston in order to fetch a slice of users
// Returns a slice of all users a user has permission to view
func (c *Client) GetUserAll() ([]User, error) {
	request := userGetAllRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetUserAll Failed")
	}

	return response.Data.GetUsers, nil
}

// RemoveWorkspaceUser sends a request to Houston in order to remove a user from a workspace
// Returns an object representing the Workspace from which the user was removed
func (c *Client) RemoveWorkspaceUser(workspaceId, email string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceUserAddRequest, workspaceId, email)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "RemoveWorkspaceUser Failed")
	}

	return response.Data.RemoveWorkspaceUser, nil
}

// UpdateDeployment sends a request to Houston in order to update the attributes of deployment
// Returns the updated Deployment object
func (c *Client) UpdateDeployment(deploymentId, jsonPayload string) (*Deployment, error) {
	request := fmt.Sprintf(deploymentUpdateRequest, deploymentId, jsonPayload)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "UpdateDeployment Failed")
	}

	return response.Data.UpdateDeployment, nil
}

// UpdateWorkspace sends a request to Houston in order to update the attributes of the workspace
// Returns an updated Workspace object
func (c *Client) UpdateWorkspace(workspaceId, jsonPayload string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceUpdateRequest, workspaceId, jsonPayload)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "UpdateWorkspace Failed")
	}

	return response.Data.UpdateWorkspace, nil
}
