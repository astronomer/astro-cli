package houston

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/pkg/errors"
	// "github.com/sirupsen/logrus"
)

var (
	createDeploymentRequest = `
	mutation CreateDeployment {
		createDeployment(
		  title: "%s",
		  organizationUuid: "",
		  teamUuid: "",
          type: "airflow",
		  version: "") {
		  success,
		  message,
		  id,
		  code
		}
	  }
	`

	createTokenRequest = `
	mutation createToken {
	  createToken(identity:"%s", password:"%s") {
	    success
	    message
	    token
	    decoded {
	      id
	      sU
	    }
	  }
	}
	`

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

	// log = logrus.WithField("package", "houston")
)

// HoustonClient containers the logger and HTTPClient used to communicate with the HoustonAPI
type HoustonClient struct {
	HTTPClient *httputil.HTTPClient
}

// NewHoustonClient returns a new Client with the logger and HTTP client setup.
func NewHoustonClient(c *httputil.HTTPClient) *HoustonClient {
	return &HoustonClient{
		HTTPClient: c,
	}
}

// GraphQLQuery wraps a graphql query string
type GraphQLQuery struct {
	Query string `json:"query"`
}

// QueryHouston executes a query against the Houston API
func (c *HoustonClient) QueryHouston(query string) (httputil.HTTPResponse, error) {
	// logger := log.WithField("function", "QueryHouston")
	doOpts := httputil.DoOptions{
		Data: GraphQLQuery{query},
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	// set headers
	if config.CFG.UserAPIAuthToken.GetString() != "" {
		doOpts.Headers["authorization"] = config.CFG.UserAPIAuthToken.GetString()
	}

	// if config.GetString(config.OrgIDCFG) != "" {
	// 	doOpts.Headers["organization"] = config.GetString(config.OrgIDCFG)
	// }

	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do("POST", config.APIURL(), &doOpts)
	if err != nil {
		return response, err
	}
	defer httpResponse.Body.Close()

	// strings.NewReader(jsonStream)
	body, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return response, err
	}

	response = httputil.HTTPResponse{
		Raw:  httpResponse,
		Body: string(body),
	}

	// logger.Debug(query)
	// logger.Debug(response.Body)

	return response, nil
}

// CreateDeployment will send request to Houston to create a new AirflowDeployment
// Returns a CreateDeploymentResponse which contains the unique id of deployment
func (c *HoustonClient) CreateDeployment(title string) (*CreateDeploymentResponse, error) {
	// logger := log.WithField("method", "CreateDeployment")
	// logger.Debug("Entered CreateDeployment")

	request := fmt.Sprintf(createDeploymentRequest, title)

	response, err := c.QueryHouston(request)
	if err != nil {
		// logger.Error(err)
		return nil, errors.Wrap(err, "CreateDeployment Failed")
	}

	var body CreateDeploymentResponse
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&body)
	if err != nil {
		// logger.Error(key)
		return nil, errors.Wrap(err, "CreateDeployment JSON decode failed")
	}
	return &body, nil
}

// CreateToken will request a new token from Houston, passing the users e-mail and password.
// Returns a CreateTokenResponse structure with the users ID and Token inside.
func (c *HoustonClient) CreateToken(email string, password string) (*CreateTokenResponse, error) {
	// logger := log.WithField("method", "CreateToken")
	// logger.Debug("Entered CreateToken")

	request := fmt.Sprintf(createTokenRequest, email, password)

	response, err := c.QueryHouston(request)
	if err != nil {
		// logger.Error(err)
		return nil, errors.Wrap(err, "CreateToken Failed")
	}

	var body CreateTokenResponse
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&body)
	if err != nil {
		// logger.Error(err)
		return nil, errors.Wrap(err, "CreateToken JSON decode failed")
	}
	return &body, nil
}

// FetchDeployments will request all airflow deployments from Houston
// Returns a FetchDeploymentResponse structure with deployment details
func (c *HoustonClient) FetchDeployments() (*FetchDeploymentsResponse, error) {
	// logger := log.WithField("method", "FetchDeployments")
	// logger.Debug("Entered FetchDeployments")

	request := fetchDeploymentsRequest

	response, err := c.QueryHouston(request)
	if err != nil {
		// logger.Error(err)
		return nil, errors.Wrap(err, "FetchDeployments Failed")
	}

	var body FetchDeploymentsResponse
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&body)
	if err != nil {
		//logger.Error(err)
		return nil, errors.Wrap(err, "FetchDeployments JSON decode failed")
	}
	return &body, nil
}
