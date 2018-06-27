package github

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/pkg/errors"
)

type RepoLatestResponse struct {
	Url     string `json:"url"`
	TagName string `json:"tag_name"`
	Draft   bool   `json:"draft"`
	// TODO created_at
}

type Client struct {
	HTTPClient *httputil.HTTPClient
}

func NewGithubClient(c *httputil.HTTPClient) *Client {
	return &Client{
		HTTPClient: c,
	}
}

func (c *Client) RepoLatestRequest(orgName string, repoName string) (*RepoLatestResponse, error) {
	doOpts := httputil.DoOptions{
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	url := "https://api.github.com/repos/%s/%s/releases/latest"
	var response httputil.HTTPResponse
	httpResponse, err := c.HTTPClient.Do("GET", fmt.Sprintf(url, orgName, repoName), &doOpts)
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

	decode := RepoLatestResponse{}
	err = json.NewDecoder(strings.NewReader(response.Body)).Decode(&decode)
	if err != nil {
		//logger.Error(err)
		return nil, errors.Wrap(err, "Failed to JSON decode Github response")
	}
	return &decode, nil
}
