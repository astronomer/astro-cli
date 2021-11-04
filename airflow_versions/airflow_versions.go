package airflowversions

import (
	"fmt"
	"sort"
	"strings"
)

var tagOrder = []string{"buster-onbuild", "onbuild", "buster"}

// GetDefaultImageTag returns default airflow image tag
func GetDefaultImageTag(httpClient *Client, airflowVersion string) (string, error) {
	r := Request{}

	resp, err := r.DoWithClient(httpClient)
	if err != nil {
		return "", err
	}

	availableTag := []string{}
	vs := make(AirflowVersions, len(resp.AvailableReleases))
	for i, r := range resp.AvailableReleases {
		if r.Version == airflowVersion {
			availableTag = r.Tags
			break
		}
		v, err := NewAirflowVersion(r.Version, r.Tags)
		if err == nil {
			vs[i] = v
		}
	}

	var selectedVersion *AirflowVersion
	if airflowVersion == "" && len(vs) != 0 {
		sort.Sort(vs)
		selectedVersion = vs[len(vs)-1]
		availableTag = selectedVersion.tags
	} else {
		selectedVersion, err = NewAirflowVersion(airflowVersion, availableTag)
		if err != nil {
			return "", err
		}
	}

	for tagIndex := range tagOrder {
		for i := range availableTag {
			if strings.HasPrefix(availableTag[i], selectedVersion.Coerce()) && strings.HasSuffix(availableTag[i], tagOrder[tagIndex]) {
				return fmt.Sprintf("%s-%s", selectedVersion.Coerce(), tagOrder[tagIndex]), nil
			}
		}
	}

	// Case when airflowVersion requested is not present in certified astronomer endpoint, but is valid version as per Houston configuration
	return airflowVersion, nil
}
