package github

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/pkg/errors"
)

var (
	http = httputil.NewHTTPClient()
	api  = NewGithubClient(http)
)

// RepoLatestResponse represents a tag info response from Github API
type RepoLatestResponse struct {
	Url         string    `json:"url"`
	TagName     string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
}

type Client struct {
	HTTPClient *httputil.HTTPClient
}

// NewGithubClient returns a HTTP client for interfacing with github
func NewGithubClient(c *httputil.HTTPClient) *Client {
	return &Client{
		HTTPClient: c,
	}
}

// GithubRequest Sends an http request to github and returns a basic response obj
func (c *Client) GithubRequest(url string, method string) (*httputil.HTTPResponse, error) {
	doOpts := httputil.DoOptions{
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do(method, url, &doOpts)
	if err != nil {
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

	return &response, nil
}

// RepoLatestRequest Makes a request to grab the latest release of a github repository
func RepoLatestRequest(orgName string, repoName string) (*RepoLatestResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", orgName, repoName)

	response, err := api.GithubRequest(url, "GET")
	if err != nil {
		return nil, err
	}

	decode := RepoLatestResponse{}
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&decode)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Failed to JSON decode Github response from %s", url))
	}
	return &decode, nil
}

// RepoTagRequest makes a request to grab a specific tag of a github repository
func RepoTagRequest(orgName string, repoName string, tagName string) (*RepoLatestResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", orgName, repoName, tagName)
	fmt.Println(url)
	response, err := api.GithubRequest(url, "GET")
	if err != nil {
		return nil, err
	}

	decode := RepoLatestResponse{}
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&decode)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Failed to JSON decode Github response from %s", url))
	}
	return &decode, nil
}
