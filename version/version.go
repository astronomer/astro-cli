package version

import (
	"context"
	"fmt"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"os"
	"runtime"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v48/github"
)

var CurrVersion string

const (
	cliCurrentVersion = "Astro CLI Version: "
)

// PrintVersion outputs current cli version and git commit if exists
func PrintVersion() {
	version := CurrVersion
	fmt.Println(cliCurrentVersion + version)
}

func getLatestRelease(client *github.Client, owner, repo string) (*github.RepositoryRelease, error) {
	ctx := context.Background()

	// Get the list of releases for the repository
	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, nil)
	if err != nil {
		return nil, err
	}

	// Find the latest release (by release tag)
	var latestRelease *github.RepositoryRelease
	for _, release := range releases {
		// Skip pre-releases
		if release.GetPrerelease() {
			continue
		}
		if latestRelease == nil || release.GetTagName() > latestRelease.GetTagName() {
			latestRelease = release
		}
	}

	return latestRelease, nil
}

func CompareVersions(client *github.Client, owner, repo string) error {
	// Get the latest release
	latestRelease, err := getLatestRelease(client, owner, repo)
	if err != nil {
		return err
	}

	// Check if current version is a local build
	if strings.Contains(CurrVersion, "SNAPSHOT") {
		return nil
	}

	// Parse the current and latest versions into semver objects
	currentSemver, err := semver.NewVersion(CurrVersion)
	if err != nil {
		return err
	}

	latestSemver, err := semver.NewVersion(latestRelease.GetTagName())
	if err != nil {
		return err
	}

	// Compare the versions and print a message to the user if the current version is outdated
	if currentSemver.LessThan(latestSemver) {
		if runtime.GOOS == "darwin" {
			fmt.Fprintf(os.Stderr, "\nA newer version of Astro CLI is available: %s\nPlease update to the latest version using 'brew upgrade astro'\n\n", latestSemver)
		} else {
			fmt.Fprintf(os.Stderr, "\nA newer version of Astro CLI is available: %s\nPlease see https://docs.astronomer.io/astro/cli/install-cli#upgrade-the-cli for information on how to update the Astro CLI\n\n", latestSemver)
		}
		fmt.Fprintf(os.Stderr, ansi.Cyan("\nTo learn more about what's new in this version, please see https://docs.astronomer.io/astro/cli/release-notes\n\n"))
		fmt.Fprintf(os.Stderr, "If you don't want to see this message again run 'astro config set -g upgrade_message false'or pass '2>/dev/null | head' to print this text to stderr\n\n")
	}

	return nil
}
