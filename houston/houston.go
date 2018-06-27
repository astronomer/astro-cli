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
			title: "%s",
			organizationUuid: "",
			teamUuid: "",
			type: "airflow",
			version: ""
		) {	
			success,
			message,
			id,
			code
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

	createTokenRequest = `
	mutation createToken {
	  createToken(
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

	fetchDeploymentsRequest = `
	query FetchAllDeployments {
	  fetchDeployments {
	    uuid
		type
		title
		release_name
		version
	  }
	}`

	fetchDeploymentRequest = `
	query FetchDeployment {
	  fetchDeployments(
			deploymentUuid: "%s"
		) {	
			uuid
			type
			title
			release_name
			version
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
	// logger := log.WithField("function", "QueryHouston")
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
	httpResponse, err := c.HTTPClient.Do("POST", config.APIURL(), &doOpts)
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

	// logger.Debug(query)
	// logger.Debug(response.Body)

	decode := HoustonResponse{}
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&decode)
	if err != nil {
		//logger.Error(err)
		return nil, errors.Wrap(err, "Failed to JSON decode Houston response")
	}

	if decode.Errors != nil {
		return nil, errors.New(decode.Errors[0].Message)
	}
	return &decode, nil
}

// CreateDeployment will send request to Houston to create a new AirflowDeployment
// Returns a StatusResponse which contains the unique id of deployment
func (c *Client) CreateDeployment(title string) (*Status, error) {
	// logger := log.WithField("method", "CreateDeployment")
	// logger.Debug("Entered CreateDeployment")

	request := fmt.Sprintf(createDeploymentRequest, title)

	response, err := c.QueryHouston(request)
	if err != nil {
		// logger.Error(err)
		return nil, errors.Wrap(err, "CreateDeployment Failed")
	}

	return response.Data.CreateDeployment, nil
}

// TODO This probably is removed in latest
// CreateToken will request a new token from Houston, passing the users e-mail and password.
// Returns a Token structure with the users ID and Token inside.
func (c *Client) CreateToken(email string, password string) (*Token, error) {
	// logger := log.WithField("method", "CreateToken")
	// logger.Debug("Entered CreateToken")

	request := fmt.Sprintf(createTokenRequest, email, password)

	response, err := c.QueryHouston(request)
	if err != nil {
		// logger.Error(err)
		return nil, errors.Wrap(err, "CreateToken Failed")
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
	fmt.Println(response)
	return response.Data.CreateUser, nil
}

// FetchDeployments will request all airflow deployments from Houston
// Returns a []Deployment structure with deployment details
func (c *Client) FetchDeployments() ([]Deployment, error) {
	// logger := log.WithField("method", "FetchDeployments")
	// logger.Debug("Entered FetchDeployments")

	request := fetchDeploymentsRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		// logger.Error(err)
		return nil, errors.Wrap(err, "FetchDeployments Failed")
	}

	return response.Data.FetchDeployments, nil
}

// FetchDeployment will request a specific airflow deployments from Houston by uuid
// Returns a Deployment structure with deployment details
func (c *Client) FetchDeployment(deploymentUuid string) (*Deployment, error) {
	// logger := log.WithField("method", "FetchDeployments")
	// logger.Debug("Entered FetchDeployments")

	request := fmt.Sprintf(fetchDeploymentRequest, deploymentUuid)

	response, err := c.QueryHouston(request)
	if err != nil {
		// logger.Error(err)
		return nil, errors.Wrap(err, "FetchDeployment Failed")
	}

	if len(response.Data.FetchDeployments) == 0 {
		return nil, fmt.Errorf("deployment not found for uuid \"%s\"", deploymentUuid)
	}
	return &response.Data.FetchDeployments[0], nil
}
