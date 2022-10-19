package sql

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type pypiVersionResponse struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
}

const astroSQLCliProjectURL = "https://pypi.org/pypi/astro-sql-cli/json"

func GetPypiVersion(projectURL string) (string, error) {
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, projectURL, http.NoBody)
	if err != nil {
		err = fmt.Errorf("Error creating HTTP request %w", err)
		return "", err
	}
	res, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("Error getting latest release version for project url %s,  %w", projectURL, err)
		return "", err
	}
	defer res.Body.Close()

	var resp pypiVersionResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		err = fmt.Errorf("Error parsing response for project version %w", err)
		return "", err
	}

	return resp.Info.Version, nil
}
