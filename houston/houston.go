package houston

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/astronomerio/astro-cli/config"
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

// Client containers the logger and HTTPClient used to communicate with the HoustonAPI
type Client struct {
	HTTPClient *HTTPClient
}

type HoustonResponse struct {
	Raw  *http.Response
	Body string
}

// NewHoustonClient returns a new Client with the logger and HTTP client setup.
func NewHoustonClient(HTTPClient *HTTPClient) *Client {
	return &Client{
		HTTPClient: HTTPClient,
	}
}

type GraphQLQuery struct {
	Query string `json:"query"`
}

func (c *Client) QueryHouston(query string) (HoustonResponse, error) {
	// logger := log.WithField("function", "QueryHouston")
	doOpts := DoOptions{
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

	var response HoustonResponse
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

	response = HoustonResponse{httpResponse, string(body)}

	// logger.Debug(query)
	// logger.Debug(response.Body)

	return response, nil
}

// CreateDeployment will send request to Houston to create a new AirflowDeployment
// Returns a CreateDeploymentResponse which contains the unique id of deployment
func (c *Client) CreateDeployment(title string) (*CreateDeploymentResponse, error) {
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
func (c *Client) CreateToken(email string, password string) (*CreateTokenResponse, error) {
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
func (c *Client) FetchDeployments() (*FetchDeploymentsResponse, error) {
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
