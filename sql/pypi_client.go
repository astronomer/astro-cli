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

func GetPypiVersion(projectURL string) string {
	httpClient := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, projectURL, http.NoBody)
	if err != nil {
		err = fmt.Errorf("error creating HTTP request %w", err)
		panic(err)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("error getting latest release version for project url %s,  %w", projectURL, err)
		panic(err)
	}
	defer res.Body.Close()

	var resp pypiVersionResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		err = fmt.Errorf("error parsing response for project version %w", err)
		panic(err)
	}

	return resp.Info.Version
}
