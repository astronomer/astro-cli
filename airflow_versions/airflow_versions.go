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

	if httpClient.useAstronomerCertified {
		return getAstronomerCertifiedTag(resp.AvailableReleases, airflowVersion)
	}

	return getAstroRuntimeTag(resp.RuntimeVersions, airflowVersion)
}

// get latest runtime tag associated to provided airflow version
// if no airflow version is provided, returns the latest astro runtime version available
func getAstroRuntimeTag(runtimeVersions map[string]RuntimeVersion, airflowVersion string) (string, error) {
	availableTags := []string{}
	availableVersions := []string{}

	for runtimeVersion, r := range runtimeVersions {
		if r.Metadata.Channel != "stable" {
			continue
		}

		// if user wants specific airflow version, get all runtime version associated to this airflow version
		if r.Metadata.AirflowVersion == airflowVersion {
			availableTags = append(availableTags, runtimeVersion)
		} else {
			availableVersions = append(availableVersions, runtimeVersion)
		}
	}

	tagsToUse := availableVersions
	if airflowVersion != "" {
		tagsToUse = availableTags
	}

	// get latest runtime version
	latestVersion, _ := NewAirflowVersion(tagsToUse[0], []string{tagsToUse[0]})
	for i := 1; i < len(tagsToUse); i++ {
		tag := tagsToUse[i]
		nextVersion, err := NewAirflowVersion(tag, []string{tag})
		if err == nil {
			if nextVersion.GreaterThan(latestVersion) {
				latestVersion = nextVersion
			}
		}
	}

	return latestVersion.Version.String(), nil
}

func getAstronomerCertifiedTag(availableReleases []AirflowVersionRaw, airflowVersion string) (string, error) {
	availableTags := []string{}
	vs := make(AirflowVersions, len(availableReleases))
	for i, r := range availableReleases {
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
	var err error
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
