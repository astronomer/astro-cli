package version

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/deployment"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/github"
	"github.com/pkg/errors"
)

var (
	CurrVersion string
	CurrCommit  string
)

// PrintVersion outputs current cli version and git commit if exists
func PrintVersion(client *houston.Client, out io.Writer) error {
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
func CheckForUpdate(client *houston.Client, ghClient *github.Client, out io.Writer) error {
	version := CurrVersion

	if !isValidVersion(version) {
		fmt.Fprintf(out, messages.CLI_UNTAGGED_PROMPT)
		fmt.Fprintf(out, messages.CLI_INSTALL_CMD)
		return nil
	}

	// fetch latest cli version
	latestTagResp, err := ghClient.RepoLatestRequest("astronomer", "astro-cli")
	if err != nil {
		fmt.Fprintln(out, err)
		latestTagResp.TagName = messages.NA
	}

	// fetch meta data around current cli version
	currentTagResp, err := ghClient.RepoTagRequest("astronomer", "astro-cli", string("v")+version)
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

	if latestTag > currentTag {
		fmt.Fprintln(out, messages.CLI_UPGRADE_PROMPT)
		fmt.Fprintln(out, messages.CLI_INSTALL_CMD)
	} else {
		fmt.Fprintln(out, messages.CLI_RUNNING_LATEST)
	}

	return nil
}

func isValidVersion(version string) bool {
	if len(version) == 0 {
		return false
	}
	return true
}

// printServerVersion outputs current server version
func printServerVersion(client *houston.Client, out io.Writer) error {
	appCfg, err := deployment.AppConfig(client)
	if err != nil {
		fmt.Fprintf(out, messages.HOUSTON_CURRENT_VERSION+"\n", "Please authenticate to a cluster to see server version")
	}

	if appCfg != nil {
		fmt.Fprintf(out, messages.HOUSTON_CURRENT_VERSION+"\n", appCfg.Version)
	}

	return nil
}
