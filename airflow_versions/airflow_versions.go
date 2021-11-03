package airflowversions

import (
	"fmt"
	"sort"
	"strings"
)

var (
	tagOrder = []string{"buster-onbuild", "onbuild", "buster"}
)

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

	selectedVersion := &AirflowVersion{}
	if len(availableTag) <= 0 || airflowVersion != "" {
		sort.Sort(vs)
		selectedVersion = vs[len(vs)-1]
		availableTag = selectedVersion.tags
	}

	for i := range availableTag {
		for tagIndex := range tagOrder {
			if strings.HasPrefix(availableTag[i], selectedVersion.Coerce()) && strings.HasSuffix(availableTag[i], tagOrder[tagIndex]) {
				return fmt.Sprintf("%s-%s", selectedVersion.Coerce(), tagOrder[tagIndex]), nil
			}
		}
	}
	return fmt.Sprintf("%s", selectedVersion.Coerce()), nil
}
