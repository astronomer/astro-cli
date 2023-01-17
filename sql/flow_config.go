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

type compatibilityVersions struct {
	AstroRuntime   string `json:"astroRuntimeVersion"`
	AstroSdkPython string `json:"astroSDKPythonVersion"`
}

type compatibilityResponse struct {
	Compatibility map[string]compatibilityVersions `json:"compatibility"`
}

const (
	defaultDockerImageURI = "quay.io/astronomer/astro-runtime:6.0.4-base"
)

var (
	getPypiVersion        = GetPypiVersion
	getBaseDockerImageURI = GetBaseDockerImageURI
)

var httpClient = &http.Client{}

func GetPypiVersion(projectURL string) (string, error) {
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
		if v, err := version.NewVersion(release); err != nil {
			fmt.Println(fmt.Errorf("error parsing release version %w", err))
		} else {
			versions[index] = v
			index++
		}
	}
	sort.Sort(sort.Reverse(version.Collection(versions)))
	return versions[0].Original(), nil
}

func GetBaseDockerImageURI(configURL string) (string, error) {
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

func GetPythonSDKComptability(configURL, sqlCliVersion string) (astroRuntimeVersion, astroSDKPythonVersion string, err error) {
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, configURL, http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("error creating HTTP request %w. Using the default", err)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("error retrieving the latest configuration %s,  %w. Using the default", configURL, err)
	}
	defer res.Body.Close()

	var resp compatibilityResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return "", "", fmt.Errorf("error parsing the compatibility versions from the configuration file: %w. Using the default", err)
	}

	SQLCLICompatibilityVersions := resp.Compatibility[sqlCliVersion]
	return SQLCLICompatibilityVersions.AstroRuntime, SQLCLICompatibilityVersions.AstroSdkPython, nil
}
