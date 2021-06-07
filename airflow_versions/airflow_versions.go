package airflowversions

import (
	"sort"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

// GetDefaultImageTag returns default airflow image tag
func GetDefaultImageTag(airflowVersion string) (string, error) {
	client := NewClient(httputil.NewHTTPClient())
	r := Request{}

	resp, err := r.DoWithClient(client)
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

	return maxAvailableVersion.tags[0], err
}
