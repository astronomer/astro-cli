package version

import (
	"fmt"
	"io"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/astrohub"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/github"
)

var (
	CurrVersion string
	CurrCommit  string
)

// PrintVersion outputs current cli version and git commit if exists
func PrintVersion(client *astrohub.Client, out io.Writer) error {
	version := CurrVersion
	gitCommit := CurrCommit

	if !isValidVersion(version) {
		return errors.New(messages.ERROR_INVALID_CLI_VERSION)
	}

	fmt.Fprintf(out, messages.CLI_CURR_VERSION+", ", version)
	fmt.Fprintf(out, messages.CLI_CURR_COMMIT+"\n", gitCommit)

	printServerVersion(client, out)

	return nil
}

// CheckForUpdate checks current version against latest on github
func CheckForUpdate(client *astrohub.Client, ghc *github.Client, out io.Writer) error {
	version := CurrVersion

	if !isValidVersion(version) {
		fmt.Fprintf(out, messages.CLI_UNTAGGED_PROMPT)
		fmt.Fprintf(out, messages.CLI_INSTALL_CMD)
		return nil
	}

	// fetch latest cli version
	latestTagResp, err := ghc.RepoLatestRequest("astronomer", "astro-cli")
	if err != nil {
		fmt.Fprintln(out, err)
		latestTagResp.TagName = messages.NA
	}

	// fetch meta data around current cli version
	currentTagResp, err := ghc.RepoTagRequest("astronomer", "astro-cli", string("v")+version)
	if err != nil {
		fmt.Fprintln(out, "Release info not found, please upgrade.")
		fmt.Fprintln(out, messages.CLI_INSTALL_CMD)
		return nil
	}

	currentPub := currentTagResp.PublishedAt.Format("2006.01.02")
	currentTag := currentTagResp.TagName
	latestPub := latestTagResp.PublishedAt.Format("2006.01.02")
	latestTag := latestTagResp.TagName

	fmt.Fprintf(out, messages.CLI_CURR_VERSION_DATE+"\n", currentTag, currentPub)
	fmt.Fprintf(out, messages.CLI_LATEST_VERSION_DATE+"\n", latestTag, latestPub)

	printServerVersion(client, out)
	compareVersions(latestTag, currentTag, out)

	return nil
}

func isValidVersion(version string) bool {
	if len(version) == 0 {
		return false
	}
	return true
}

// printServerVersion outputs current server version
func printServerVersion(client *astrohub.Client, out io.Writer) error {
	appCfg, err := deployment.AppConfig(client)
	if err != nil {
		fmt.Fprintf(out, messages.HOUSTON_CURRENT_VERSION+"\n", "Please authenticate to a cluster to see server version")
	}

	if appCfg != nil {
		fmt.Fprintf(out, messages.HOUSTON_CURRENT_VERSION+"\n", appCfg.Version)
	}

	return nil
}
