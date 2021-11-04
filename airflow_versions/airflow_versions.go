package airflowversions

import (
	"fmt"
	"sort"
	"strings"
)

var tagPrefixOrder = []string{"buster-onbuild", "onbuild", "buster"}

// GetDefaultImageTag returns default airflow image tag
func GetDefaultImageTag(httpClient *Client, airflowVersion string) (string, error) {
	r := Request{}

	resp, err := r.DoWithClient(httpClient)
	if err != nil {
		return "", err
	}

	availableTags := []string{}
	vs := make(AirflowVersions, len(resp.AvailableReleases))
	for i, r := range resp.AvailableReleases {
		if r.Version == airflowVersion {
			availableTags = r.Tags
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
		availableTags = selectedVersion.tags
	} else {
		selectedVersion, err = NewAirflowVersion(airflowVersion, availableTags)
		if err != nil {
			return "", err
		}
	}

	for tagIndex := range tagPrefixOrder {
		for idx := range availableTags {
			if strings.HasPrefix(availableTags[idx], selectedVersion.Coerce()) && strings.HasSuffix(availableTags[idx], tagPrefixOrder[tagIndex]) {
				return fmt.Sprintf("%s-%s", selectedVersion.Coerce(), tagPrefixOrder[tagIndex]), nil
			}
		}
	}

	// case when airflowVersion requested is not present in certified astronomer endpoint, but is valid version as per Houston configuration
	return airflowVersion, nil
}
