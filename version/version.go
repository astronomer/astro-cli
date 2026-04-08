package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	semver "github.com/Masterminds/semver/v3"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

var CurrVersion string

const (
	cliCurrentVersion  = "Astro CLI Version: "
	astroCLIReleaseURL = "https://updates.astronomer.io/astro-cli"
)

type astroCLIRelease struct {
	Version string `json:"version"`
}

type astroCLIReleaseResponse struct {
	AvailableReleases []astroCLIRelease `json:"available_releases"`
}

// PrintVersion outputs current cli version and git commit if exists
func PrintVersion() {
	version := CurrVersion
	fmt.Println(cliCurrentVersion + version)
}

func getCLIReleases(ctx context.Context, client *http.Client, url string) (*astroCLIReleaseResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("astro-cli releases endpoint request failed with status: %d", response.StatusCode)
	}

	var releases astroCLIReleaseResponse
	err = json.NewDecoder(response.Body).Decode(&releases)
	if err != nil {
		return nil, err
	}

	return &releases, nil
}

func getLatestRelease(ctx context.Context, client *http.Client, url string) (*semver.Version, error) {
	releases, err := getCLIReleases(ctx, client, url)
	if err != nil {
		return nil, err
	}

	if len(releases.AvailableReleases) == 0 {
		return nil, errors.New("astro-cli releases endpoint returned 0 results")
	}

	var latest *semver.Version
	for _, r := range releases.AvailableReleases {
		// discard any versions that we cannot parse
		v, err := semver.NewVersion(r.Version)
		if err == nil && (latest == nil || v.GreaterThan(latest)) {
			latest = v
		}
	}

	if latest == nil {
		return nil, errors.New("astro-cli releases endpoint returned 0 valid versions")
	}

	return latest, nil
}

func CompareVersions(ctx context.Context, client *http.Client) error {
	// Check if current version is a local build
	if strings.Contains(CurrVersion, "SNAPSHOT") {
		return nil
	}

	// Get the latest release
	latestSemver, err := getLatestRelease(ctx, client, astroCLIReleaseURL)
	if err != nil {
		return err
	}

	// Parse the current and latest versions into semver objects
	currentSemver, err := semver.NewVersion(CurrVersion)
	if err != nil {
		return err
	}

	// Compare the versions and print a message to the user if the current version is outdated
	if currentSemver.LessThan(latestSemver) {
		fmt.Fprintf(os.Stderr, "\nA newer version of Astro CLI is available: %s\nPlease see https://www.astronomer.io/docs/astro/cli/upgrade-cli for information on how to update the Astro CLI\n\n", latestSemver)
		fmt.Fprint(os.Stderr, ansi.Cyan("\nTo learn more about what's new in this version, please see https://www.astronomer.io/docs/astro/cli/release-notes\n\n"))
		fmt.Fprintf(os.Stderr, "If you don't want to see this message again run 'astro config set -g upgrade_message false' or pass '2>/dev/null' to print this text to stderr\n\n")
	}

	return nil
}
