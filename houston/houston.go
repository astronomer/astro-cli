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
	createDeploymentRequest = `
	mutation CreateDeployment {
		createDeployment(	
			label: "%s",
			type: "airflow",
			teamUuid: "%s"
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

	createWorkspaceRequest = `
	mutation CreateWorkspace {
		createTeam(
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

	createUserRequest = `
	mutation CreateUser {
		createUser(
			email: "%s",
			password: "%s"
		) {
			success,
			message,
			token
		}
	}`

	createBasicTokenRequest = `
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

	createOAuthTokenRequest = `
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

	deleteDeploymentRequest = `
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

	deleteWorkspaceRequest = `
	mutation DeleteWorkspace {
		deleteTeam(teamUuid: "%s") {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`

	getDeploymentsRequest = `
	query GetDeployments {
	  deployments(teamUuid: "%s") {
		uuid
		type
		label
		releaseName
		version
		createdAt
		updatedAt
	  }
	}`

	getDeploymentRequest = `
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

	getAuthConfigRequest = `
	query GetAuthConfig {
		authConfig(state: "cli") {
		  localEnabled
		  googleEnabled
		  googleOAuthUrl
		}
	  }`

	getWorkspaceAllRequest = `
	query GetWorkspaces {
		teams {
			uuid
			label
			description
			active
			createdAt
			updatedAt
		}
	}`
	// log = logrus.WithField("package", "houston")
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

	// if config.GetString(config.OrgIDCFG) != "" {
	// 	doOpts.Headers["organization"] = config.GetString(config.OrgIDCFG)
	// }

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

// CreateDeployment will send request to Houston to create a new AirflowDeployment
// Returns a StatusResponse which contains the unique id of deployment
func (c *Client) CreateDeployment(title, wsId string) (*Deployment, error) {
	request := fmt.Sprintf(createDeploymentRequest, title, wsId)
	fmt.Println(request)
	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateDeployment Failed")
	}

	return response.Data.CreateDeployment, nil
}

// CreateBasicToken will request a new token from Houston, passing the users e-mail and password.
// Returns a Token structure with the users ID and Token inside.
func (c *Client) CreateBasicToken(email string, password string) (*AuthUser, error) {
	request := fmt.Sprintf(createBasicTokenRequest, email, password)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateBasicToken Failed")
	}

	return response.Data.CreateToken, nil
}

// CreateOAuthToken passes an OAuth type and authCode to createOauthTokenRequest in order allow houston to authenticate user
// Returns a Token structure with the users ID and Token inside.
func (c *Client) CreateOAuthToken(authCode string) (*AuthUser, error) {
	request := fmt.Sprintf(createOAuthTokenRequest, "GOOGLE_OAUTH", authCode)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateOAuthToken Failed")
	}

	return response.Data.CreateToken, nil
}

// CreateUser will send a request to houston to create a new user
// Returns a Status object with a new token
func (c *Client) CreateUser(email string, password string) (*Token, error) {
	request := fmt.Sprintf(createUserRequest, email, password)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateUser Failed")
	}
	return response.Data.CreateUser, nil
}

// CreateWorkspace will send a request to houston to create a new workspace
// Returns an object representing created workspace
func (c *Client) CreateWorkspace(label, description string) (*Workspace, error) {
	request := fmt.Sprintf(createWorkspaceRequest, label, description)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "CreateWorkspace Failed")
	}

	return response.Data.CreateWorkspace, nil
}

func (c *Client) DeleteDeployment(uuid string) (*Deployment, error) {
	request := fmt.Sprintf(deleteDeploymentRequest, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "DeleteDeployment Failed")
	}

	return response.Data.DeleteDeployment, nil
}

// DeleteWorkspace will send a request to houston to create a new workspace
// Returns an object representing deleted workspace
func (c *Client) DeleteWorkspace(uuid string) (*Workspace, error) {
	request := fmt.Sprintf(deleteWorkspaceRequest, uuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "DeleteWorkspace Failed")
	}

	return response.Data.DeleteWorkspace, nil
}

// GetDeployments will request all airflow deployments from Houston
// Returns a []Deployment structure with deployment details
func (c *Client) GetDeployments(ws string) ([]Deployment, error) {
	request := fmt.Sprintf(getDeploymentsRequest, ws)

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetDeployments Failed")
	}

	return response.Data.GetDeployments, nil
}

// GetDeployment will request a specific airflow deployments from Houston by uuid
// Returns a Deployment structure with deployment details
func (c *Client) GetDeployment(deploymentUuid string) (*Deployment, error) {
	request := fmt.Sprintf(getDeploymentRequest, deploymentUuid)

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
	request := getAuthConfigRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetAuthConfig Failed")
	}

	return response.Data.GetAuthConfig, nil
}

// GetWorkspaceAll returns all available workspaces from houston API
func (c *Client) GetWorkspaceAll() ([]Workspace, error) {
	request := getWorkspaceAllRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		return nil, errors.Wrap(err, "GetWorkspaceAll Failed")
	}

	return response.Data.GetWorkspace, nil
}
