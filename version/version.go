package version

import (
	"errors"
	"fmt"

	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/github"
)

var (
	api = github.NewGithubClient()
)

// PrintVersion outputs current cli version and git commit if exists
func PrintVersion(version, gitCommit string) error {
	if !isValidVersion(version) {
		return errors.New(messages.ERROR_INVALID_CLI_VERSION)
	}

	fmt.Printf(messages.CLI_CURR_VERSION+"\n", version)
	fmt.Printf(messages.CLI_CURR_COMMIT+"\n", gitCommit)
	return nil
}

// CheckForUpdate checks current version against latest on github
func CheckForUpdate(version, gitCommit string) error {
	if !isValidVersion(version) {
		fmt.Println(messages.CLI_UNTAGGED_PROMPT)
		fmt.Println(messages.CLI_INSTALL_CMD)
		return nil
	}

	// fetch latest cli version
	latestTagResp, err := api.RepoLatestRequest("astronomerio", "astro-cli")
	if err != nil {
		fmt.Println(err)
		latestTagResp.TagName = messages.NA
	}

	// fetch meta data around current cli version
	currentTagResp, err := api.RepoTagRequest("astronomerio", "astro-cli", string("v")+version)
	if err != nil {
		fmt.Println(err)
	}

	currentPub := currentTagResp.PublishedAt.Format("2006.01.02")
	latestPub := latestTagResp.PublishedAt.Format("2006.01.02")
	latestTag := latestTagResp.TagName

	if latestTagResp.TagName > version {
		fmt.Printf(messages.CLI_CURR_VERSION_DATE+"\n", version, currentPub)
		fmt.Printf(messages.CLI_LATEST_VERSION_DATE+"\n", latestTag, latestPub)
		fmt.Println(messages.CLI_UPGRADE_PROMPT)
		fmt.Println(messages.CLI_INSTALL_CMD)
		return nil
	}

	return nil
}

func isValidVersion(version string) bool {
	if len(version) == 0 {
		return false
	}
	return true
}
