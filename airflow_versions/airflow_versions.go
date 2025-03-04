package airflowversions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/astronomer/astro-cli/pkg/logger"
)

const (
	VersionChannelStable  = "stable"
	DefaultRuntimeVersion = "5.0.1"
	DefaultAirflowVersion = "2.3.0-onbuild"
)

var tagPrefixOrder = []string{"buster-onbuild", "onbuild", "buster"}

type ErrNoTagAvailable struct {
	airflowVersion string
}

func (e ErrNoTagAvailable) Error() string {
	return fmt.Sprintf("there is no tag available for provided airflow version: %s, you might want to try a different airflow version.", e.airflowVersion)
}

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

	return getAstroRuntimeTag(resp.RuntimeVersions, resp.RuntimeVersionsV3, airflowVersion)
}

// get latest runtime tag associated to provided airflow version or directly runtimeVersion
// if no airflow version is provided, returns the latest astro runtime version available
func getAstroRuntimeTag(runtimeVersions, runtimeVersionsV3 map[string]RuntimeVersion, airflowVersion string) (string, error) {
	availableVersions := []string{}

	for runtimeVersion, r := range runtimeVersions {
		if r.Metadata.Channel != VersionChannelStable {
			continue
		}
		if airflowVersion != "" && r.Metadata.AirflowVersion != airflowVersion {
			continue
		}
		availableVersions = append(availableVersions, runtimeVersion)
	}
	for runtimeVersion, r := range runtimeVersionsV3 {
		if r.Metadata.Channel != VersionChannelStable {
			continue
		}
		if airflowVersion != "" && r.Metadata.AirflowVersion != airflowVersion {
			continue
		}
		availableVersions = append(availableVersions, runtimeVersion)
	}

	logger.Debugf("Available runtime versions: %v", availableVersions)

	if airflowVersion != "" && len(availableVersions) == 0 {
		return "", ErrNoTagAvailable{airflowVersion: airflowVersion}
	}

	latestVersion := availableVersions[0]
	for _, availableVersion := range availableVersions {
		if CompareRuntimeVersions(availableVersion, latestVersion) > 0 {
			latestVersion = availableVersion
		}
	}

	logger.Debugf("Latest runtime version: %s", latestVersion)

	return latestVersion, nil
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
