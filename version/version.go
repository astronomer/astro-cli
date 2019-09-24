package version

import (
	"errors"
	"fmt"
	"io"
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
func PrintVersion(out io.Writer) error {
	version := CurrVersion
	gitCommit := CurrCommit

	if !isValidVersion(version) {
		return errors.New(messages.ERROR_INVALID_CLI_VERSION)
	}

	fmt.Fprintf(out, messages.CLI_CURR_VERSION+"\n", version)
	fmt.Fprintf(out,messages.CLI_CURR_COMMIT+"\n", gitCommit)
	return nil
}

// CheckForUpdate checks current version against latest on github
func CheckForUpdate(out io.Writer) error {
	version := CurrVersion

	if !isValidVersion(version) {
		fmt.Fprintf(out, messages.CLI_UNTAGGED_PROMPT)
		fmt.Fprintf(out, messages.CLI_INSTALL_CMD)
		return nil
	}

	// fetch latest cli version
	latestTagResp, err := api.RepoLatestRequest("astronomer", "astro-cli")
	if err != nil {
		fmt.Fprintln(out, err)
		latestTagResp.TagName = messages.NA
	}

	// fetch meta data around current cli version
	currentTagResp, err := api.RepoTagRequest("astronomer", "astro-cli", string("v")+version)
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

func GetTagFromVersion(airflowVersion string) string {

	if airflowVersion == "" {
		airflowVersion = "1.10.5"
	}

	version := CurrVersion

	if !isValidVersion(version) || s.HasPrefix(version, "SNAPSHOT-") {
		return fmt.Sprintf("master-%s-onbuild", airflowVersion)
	} else {
		return fmt.Sprintf("%s-%s-onbuild", version, airflowVersion)
	}
}
