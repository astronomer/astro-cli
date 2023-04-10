package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

type configResponse struct {
	BaseDockerImage string `json:"baseDockerImage"`
}

type compatibilityVersions struct {
	AstroRuntime   string `json:"astroRuntimeVersion"`
	AstroSdkPython string `json:"astroSDKPythonVersion"`
}

type sqlCliCompatibilityVersions struct {
	SQLCliVersion string `json:"sqlCliVersion"`
	PreRelease    bool   `json:"preRelease"`
}

type compatibilityResponse struct {
	Compatibility       map[string]compatibilityVersions       `json:"compatibility"`
	SQLCliCompatibility map[string]sqlCliCompatibilityVersions `json:"astroCliCompatibility"`
}

const (
	defaultDockerImageURI = "quay.io/astronomer/astro-runtime:6.0.4-base"
)

var (
	getPypiVersion           = GetPypiVersion
	getBaseDockerImageURI    = GetBaseDockerImageURI
	getPythonSDKComptability = GetPythonSDKComptability
)

var httpClient = &http.Client{}

var semVerRegex = regexp.MustCompile(`^(\d+)[.](\d+)[.](\d+)$`)

const (
	trimmedMinorVersionRegexMatchString = "$1.$2"
	defaultVersionString                = "default"
)

type AstroSQLCliVersion struct {
	Version    string
	Prerelease bool
}

func GetPypiVersion(configURL, astroCliVersion string) (AstroSQLCliVersion, error) {
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, configURL, http.NoBody)
	if err != nil {
		return AstroSQLCliVersion{}, fmt.Errorf("error creating HTTP request %w", err)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return AstroSQLCliVersion{}, fmt.Errorf("error getting latest release version for project url %s,  %w", configURL, err)
	}
	defer res.Body.Close()

	var resp compatibilityResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return AstroSQLCliVersion{}, fmt.Errorf("error parsing response for project version %w", err)
	}
	SQLCliCompatibility, exists := resp.SQLCliCompatibility[astroCliVersion]
	if !exists {
		astroCliMinorVersion := semVerRegex.ReplaceAllString(astroCliVersion, trimmedMinorVersionRegexMatchString)
		SQLCliCompatibility, exists = resp.SQLCliCompatibility[astroCliMinorVersion]
		if !exists {
			SQLCliCompatibility, exists = resp.SQLCliCompatibility[defaultVersionString]
			if !exists {
				return AstroSQLCliVersion{}, fmt.Errorf("error parsing response for SQL CLI version %w", err)
			}
		}
	}
	return AstroSQLCliVersion{SQLCliCompatibility.SQLCliVersion, SQLCliCompatibility.PreRelease}, nil
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
	if SQLCLICompatibilityVersions.AstroRuntime == "" {
		return "", "", fmt.Errorf("could not find a matching SQL CLI compatibility version: current version %s version map: %v  %w", sqlCliVersion, resp.Compatibility, err)
	}
	return SQLCLICompatibilityVersions.AstroRuntime, SQLCLICompatibilityVersions.AstroSdkPython, nil
}
