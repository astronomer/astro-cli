package airflowversions

import (
	"fmt"
	"regexp"
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

// imageTagRegex matches image tags like "3.1-1-python-3.12-astro-agent-1.1.0"
var imageTagRegex = regexp.MustCompile(`^(?P<runtime>\d+\.\d+(?:-\d+)?)-python-(?P<python>\d+\.\d+)-astro-agent-(?P<agent>.+?)(?:-base)?$`)

type ErrNoTagAvailable struct {
	airflowVersion string
}

func (e ErrNoTagAvailable) Error() string {
	return fmt.Sprintf("there is no tag available for provided airflow version: %s, you might want to try a different airflow version.", e.airflowVersion)
}

// ImageTagInfo holds parsed information from an image tag
type ImageTagInfo struct {
	RuntimeVersion string
	PythonVersion  string
	AgentVersion   string
	IsBase         bool
}

// parseImageTag extracts version information from an image tag using regex
func parseImageTag(imageTag string) (*ImageTagInfo, error) {
	// Check if it's a base image
	isBase := strings.HasSuffix(imageTag, "-base")

	// Parse using regex
	matches := imageTagRegex.FindStringSubmatch(imageTag)
	if matches == nil {
		return nil, fmt.Errorf("image tag does not match expected pattern: %s", imageTag)
	}

	// Extract named groups
	result := make(map[string]string)
	for i, name := range imageTagRegex.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}

	return &ImageTagInfo{
		RuntimeVersion: result["runtime"],
		PythonVersion:  result["python"],
		AgentVersion:   result["agent"],
		IsBase:         isBase,
	}, nil
}

// isBetterImage compares two ImageTagInfo objects to determine if the candidate is better
// Priority: Runtime version first, then Python version
func isBetterImage(candidate, current *ImageTagInfo) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}

	// Compare runtime versions first using existing function
	runtimeComparison := CompareRuntimeVersions(candidate.RuntimeVersion, current.RuntimeVersion)
	if runtimeComparison > 0 {
		return true
	}
	if runtimeComparison < 0 {
		return false
	}

	// Same runtime version, compare Python versions (string comparison works for semantic versions like "3.11" vs "3.12")
	return candidate.PythonVersion > current.PythonVersion
}

// GetDefaultImageTag returns default airflow image tag
func GetDefaultImageTag(httpClient *Client, airflowVersion string, excludeAirflow3 bool) (string, error) {
	r := Request{}

	resp, err := r.DoWithClient(httpClient)
	if err != nil {
		return "", err
	}

	if httpClient.useAstronomerCertified {
		return getAstronomerCertifiedTag(resp.AvailableReleases, airflowVersion)
	}

	if httpClient.useAstroAgent {
		return getAstroAgentTag(resp.ClientVersions)
	}

	if excludeAirflow3 {
		return getAstroRuntimeTag(resp.RuntimeVersions, nil, airflowVersion)
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

	if len(availableVersions) == 0 {
		if airflowVersion != "" {
			return "", ErrNoTagAvailable{airflowVersion: airflowVersion}
		} else {
			return "", fmt.Errorf("no runtime versions found")
		}
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

// get latest client tag
func getAstroAgentTag(clientVersions map[string]ClientVersion) (string, error) {
	if len(clientVersions) == 0 {
		return "", fmt.Errorf("no client versions found")
	}

	availableVersions := []string{}
	for clientVersion, c := range clientVersions {
		if c.Metadata.Channel != VersionChannelStable {
			continue
		}
		availableVersions = append(availableVersions, clientVersion)
	}

	logger.Debugf("Available client versions: %v", availableVersions)

	if len(availableVersions) == 0 {
		return "", fmt.Errorf("no client versions found")
	}

	latestVersion := availableVersions[0]
	for _, availableVersion := range availableVersions {
		if CompareRuntimeVersions(availableVersion, latestVersion) > 0 {
			latestVersion = availableVersion
		}
	}

	logger.Debugf("Latest client version: %s", latestVersion)

	// Parse and filter image tags, prioritizing latest Python version
	var bestImageTag string
	var bestInfo *ImageTagInfo

	for _, imageTags := range clientVersions[latestVersion].ImageTags {
		tagInfo, err := parseImageTag(imageTags)
		if err != nil {
			logger.Debugf("Failed to parse image tag %s: %v", imageTags, err)
			continue
		}

		// Skip -base images
		if tagInfo.IsBase {
			continue
		}

		// Select the best image based on runtime version first, then Python version
		if bestImageTag == "" || isBetterImage(tagInfo, bestInfo) {
			bestImageTag = imageTags
			bestInfo = tagInfo
		}
	}

	if bestImageTag == "" {
		return "", fmt.Errorf("no non-base images found for client version %s", latestVersion)
	}

	logger.Debugf("Latest image tag: %s", bestImageTag)

	return bestImageTag, nil
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
