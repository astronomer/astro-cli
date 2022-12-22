package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/hashicorp/go-version"
)

type pypiVersionResponse struct {
	Releases map[string]json.RawMessage `json:"releases"`
}

type configResponse struct {
	BaseDockerImage string `json:"baseDockerImage"`
}

const (
	defaultDockerImageURI = "quay.io/astronomer/astro-runtime:6.0.4-base"
)

var (
	getPypiVersion        = GetPypiVersion
	getBaseDockerImageURI = GetBaseDockerImageURI
)

func GetPypiVersion(projectURL string) (string, error) {
	httpClient := &http.Client{}
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, projectURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("error creating HTTP request %w", err)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error getting latest release version for project url %s,  %w", projectURL, err)
	}
	defer res.Body.Close()

	var resp pypiVersionResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return "", fmt.Errorf("error parsing response for project version %w", err)
	}

	versions := make([]*version.Version, len(resp.Releases))
	index := 0
	for release := range resp.Releases {
		versions[index], err = version.NewVersion(release)
		if err != nil {
			return "", fmt.Errorf("error parsing release version %w", err)
		}
		index++
	}
	sort.Sort(sort.Reverse(version.Collection(versions)))
	return versions[0].Original(), nil
}

func GetBaseDockerImageURI(configURL string) (string, error) {
	httpClient := &http.Client{}
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, configURL, http.NoBody)
	if err != nil {
		return defaultDockerImageURI, fmt.Errorf("error creating HTTP request %w. Using the default", err)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return defaultDockerImageURI, fmt.Errorf("error retrieving the latest configuration %s,  %w. Using the default", configURL, err)
	}
	defer res.Body.Close()

	var resp configResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return defaultDockerImageURI, fmt.Errorf("error parsing the base docker image from the configuration file: %w. Using the default", err)
	}

	return resp.BaseDockerImage, nil
}
