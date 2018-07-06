package houston

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
	// "github.com/sirupsen/logrus"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/pkg/httputil"
)

var (
	authConfigGetRequest = `
	query GetAuthConfig {
		authConfig(state: "cli") {
		  localEnabled
		  googleEnabled
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
		version
		createdAt
		updatedAt
	  }
	}`

	tokenBasicCreateRequest = `
	mutation createBasicToken {
	  createToken(
		  authStrategy:LOCAL
		  identity:"%s",
		  password:"%s"
		) {
			success
			message
			token
			decoded {
	      id
	      sU
	    }
	  }
	}`

	tokenOAuthCreateRequest = `
	mutation createOauthBasicToken {
	  createToken(
		authStrategy:%s
		credentials:"%s"
		) { 
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

// QueryHouston executes a query against the Houston API
func (c *Client) QueryHouston(query string) (*HoustonResponse, error) {
	doOpts := httputil.DoOptions{
		Data: GraphQLQuery{query},
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	// set headers
	if config.CFG.CloudAPIToken.GetString() != "" {
		doOpts.Headers["authorization"] = config.CFG.CloudAPIToken.GetString()
	}

	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do("POST", config.APIUrl(), &doOpts)
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

	if decode.Errors != nil {
		return nil, errors.New(decode.Errors[0].Message)
	}
	return &decode, nil
}

func (c *Client) AddWorkspaceUser(workspaceId, email string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceUserAddRequest, workspaceId, email)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "AddWorkspaceUser Failed")
	}

	return response.Data.AddWorkspaceUser, nil
}

// CreateDeployment will send request to Houston to create a new AirflowDeployment
// Returns a StatusResponse which contains the unique id of deployment
func (c *Client) CreateDeployment(label, wsId string) (*Deployment, error) {
	request := fmt.Sprintf(deploymentCreateRequest, label, wsId)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateDeployment Failed")
	}

	return response.Data.CreateDeployment, nil
}

// CreateBasicToken will request a new token from Houston, passing the users e-mail and password.
// Returns a Token structure with the users ID and Token inside.
func (c *Client) CreateBasicToken(email string, password string) (*AuthUser, error) {
	request := fmt.Sprintf(tokenBasicCreateRequest, email, password)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateBasicToken Failed")
	}

	return response.Data.CreateToken, nil
}

// CreateOAuthToken passes an OAuth type and authCode to createOauthTokenRequest in order allow houston to authenticate user
// Returns a Token structure with the users ID and Token inside.
func (c *Client) CreateOAuthToken(authCode string) (*AuthUser, error) {
	request := fmt.Sprintf(tokenOAuthCreateRequest, "GOOGLE_OAUTH", authCode)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateOAuthToken Failed")
	}

	return response.Data.CreateToken, nil
}

// CreateUser will send a request to houston to create a new user
// Returns a Status object with a new token
func (c *Client) CreateUser(email string, password string) (*AuthUser, error) {
	request := fmt.Sprintf(userCreateRequest, email, password)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateUser Failed")
	}
	return response.Data.CreateUser, nil
}

// CreateWorkspace will send a request to houston to create a new workspace
// Returns an object representing created workspace
func (c *Client) CreateWorkspace(label, description string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceCreateRequest, label, description)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateWorkspace Failed")
	}

	return response.Data.CreateWorkspace, nil
}

func (c *Client) DeleteDeployment(uuid string) (*Deployment, error) {
	request := fmt.Sprintf(deploymentDeleteRequest, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "DeleteDeployment Failed")
	}

	return response.Data.DeleteDeployment, nil
}

// DeleteWorkspace will send a request to houston to create a new workspace
// Returns an object representing deleted workspace
func (c *Client) DeleteWorkspace(uuid string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceDeleteRequest, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "DeleteWorkspace Failed")
	}

	return response.Data.DeleteWorkspace, nil
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
func (c *Client) GetDeployment(deploymentUuid string) (*Deployment, error) {
	request := fmt.Sprintf(deploymentGetRequest, deploymentUuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetDeployment Failed")
	}

	if len(response.Data.GetDeployments) == 0 {
		return nil, fmt.Errorf("deployment not found for uuid \"%s\"", deploymentUuid)
	}
	return &response.Data.GetDeployments[0], nil
}

// GetAuthConfig will get authentication configuration from houston
func (c *Client) GetAuthConfig() (*AuthConfig, error) {
	request := authConfigGetRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetAuthConfig Failed")
	}

	return response.Data.GetAuthConfig, nil
}

// GetWorkspaceAll returns all available workspaces from houston API
func (c *Client) GetWorkspaceAll() ([]Workspace, error) {
	request := workspaceAllGetRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetWorkspaceAll Failed")
	}

	return response.Data.GetWorkspace, nil
}

func (c *Client) GetUserAll() ([]User, error) {
	request := userGetAllRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetUserAll Failed")
	}

	return response.Data.GetUsers, nil
}

func (c *Client) RemoveWorkspaceUser(workspaceId, email string) (*Workspace, error) {
	request := fmt.Sprintf(workspaceUserAddRequest, workspaceId, email)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "RemoveWorkspaceUser Failed")
	}

	return response.Data.RemoveWorkspaceUser, nil
}
