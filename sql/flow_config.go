package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type pypiVersionResponse struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
}

type configResponse struct {
	BaseDockerImage string `json:"baseDockerImage"`
}

const (
	astroSQLCliProjectURL = "https://pypi.org/pypi/astro-sql-cli/json"
	astroSQLCliConfigURL  = "https://raw.githubusercontent.com/astronomer/astro-sdk/astro-cli/sql-cli/config/astro-cli.json"
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
		err = fmt.Errorf("error creating HTTP request %w", err)
		return "", err
	}
	res, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("error getting latest release version for project url %s,  %w", projectURL, err)
		return "", err
	}
	defer res.Body.Close()

	var resp pypiVersionResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		err = fmt.Errorf("error parsing response for project version %w", err)
		return "", err
	}

	return resp.Info.Version, nil
}

func GetBaseDockerImageURI(configURL string) (string, error) {
	httpClient := &http.Client{}
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, configURL, http.NoBody)
	if err != nil {
		err = fmt.Errorf("error creating HTTP request %w. Using the default", err)
		return defaultDockerImageURI, err
	}
	res, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("error retrieving the latest configuration %s,  %w. Using the default", configURL, err)
		return defaultDockerImageURI, err
	}
	defer res.Body.Close()

	var resp configResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		err = fmt.Errorf("error parsing the base docker image from the configuration file: %w", err)
		return defaultDockerImageURI, err
	}

	return resp.BaseDockerImage, nil
}
