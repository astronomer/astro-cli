package airflowversions

import (
	"fmt"
	"sort"
)

// GetDefaultImageTag returns default airflow image tag
func GetDefaultImageTag(httpClient *Client, airflowVersion string) (string, error) {
	r := Request{}

	resp, err := r.DoWithClient(httpClient)
	if err != nil {
		return "", err
	}

	vs := make(AirflowVersions, len(resp.AvailableReleases))
	for i, r := range resp.AvailableReleases {
		v, _ := NewAirflowVersion(r.Version, r.Tags)
		vs[i] = v
	}

	sort.Sort(vs)
	maxAvailableVersion := vs[len(vs)-1]
	return fmt.Sprintf("%s-buster-onbuild", maxAvailableVersion.Coerce()), nil
}
