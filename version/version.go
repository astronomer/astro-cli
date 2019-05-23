package version

import (
	"errors"
	"fmt"
	s "strings"

	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/github"
)

var (
	CurrVersion string
	CurrCommit  string
	api         = github.NewGithubClient()
)

// PrintVersion outputs current cli version and git commit if exists
func PrintVersion() error {
	version := CurrVersion
	gitCommit := CurrCommit

	if !isValidVersion(version) {
		return errors.New(messages.ERROR_INVALID_CLI_VERSION)
	}

	fmt.Printf(messages.CLI_CURR_VERSION+"\n", version)
	fmt.Printf(messages.CLI_CURR_COMMIT+"\n", gitCommit)
	return nil
}

// CheckForUpdate checks current version against latest on github
func CheckForUpdate() error {
	version := CurrVersion

	if !isValidVersion(version) {
		fmt.Println(messages.CLI_UNTAGGED_PROMPT)
		fmt.Println(messages.CLI_INSTALL_CMD)
		return nil
	}

	// fetch latest cli version
	latestTagResp, err := api.RepoLatestRequest("astronomer", "astro-cli")
	if err != nil {
		fmt.Println(err)
		latestTagResp.TagName = messages.NA
	}

	// fetch meta data around current cli version
	currentTagResp, err := api.RepoTagRequest("astronomer", "astro-cli", string("v")+version)
	if err != nil {
		fmt.Println("Release info not found, please upgrade.")
		fmt.Println(messages.CLI_INSTALL_CMD)
		return nil
	}

	currentPub := currentTagResp.PublishedAt.Format("2006.01.02")
	currentTag := currentTagResp.TagName
	latestPub := latestTagResp.PublishedAt.Format("2006.01.02")
	latestTag := latestTagResp.TagName

	fmt.Printf(messages.CLI_CURR_VERSION_DATE+"\n", currentTag, currentPub)
	fmt.Printf(messages.CLI_LATEST_VERSION_DATE+"\n", latestTag, latestPub)

	if latestTag > currentTag {
		fmt.Println(messages.CLI_UPGRADE_PROMPT)
		fmt.Println(messages.CLI_INSTALL_CMD)
	} else {
		fmt.Println(messages.CLI_RUNNING_LATEST)
	}

	return nil
}

func isValidVersion(version string) bool {
	if len(version) == 0 {
		return false
	}
	return true
}

func GetTagFromVersion(airflowVersion string) string {

	if airflowVersion == "" {
		airflowVersion = "1.10.3"
	}

	version := CurrVersion

	if !isValidVersion(version) || s.HasPrefix(version, "SNAPSHOT-") {
		return fmt.Sprintf("master-onbuild-%s", airflowVersion)
	} else {
		return fmt.Sprintf("%s-%s-onbuild", version, airflowVersion)
	}
}
