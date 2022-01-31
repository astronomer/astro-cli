package version

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/github"
)

var (
	CurrVersion string
	CurrCommit  string
)

var errInvalidCLIVersion = errors.New(messages.ErrInvalidCLIVersion)

// PrintVersion outputs current cli version and git commit if exists
func PrintVersion(client houston.HoustonClientInterface, out io.Writer) error {
	version := CurrVersion
	gitCommit := CurrCommit

	if !isValidVersion(version) {
		return errInvalidCLIVersion
	}

	fmt.Fprintf(out, messages.CLICurrVersion+", ", version)
	fmt.Fprintf(out, messages.CLICurrCommit+"\n", gitCommit)

	printServerVersion(client, out)

	return nil
}

// CheckForUpdate checks current version against latest on github
func CheckForUpdate(client houston.HoustonClientInterface, ghc *github.Client, out io.Writer) error {
	version := CurrVersion

	if !isValidVersion(version) {
		fmt.Fprintf(out, messages.CLIUntaggedPrompt)
		fmt.Fprintf(out, messages.CLIInstallCMD)
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
		fmt.Fprintln(out, messages.CLIInstallCMD)
		return nil
	}

	currentPub := currentTagResp.PublishedAt.Format("2006.01.02")
	currentTag := currentTagResp.TagName
	latestPub := latestTagResp.PublishedAt.Format("2006.01.02")
	latestTag := latestTagResp.TagName

	fmt.Fprintf(out, messages.CLICurrVersionDate+"\n", currentTag, currentPub)
	fmt.Fprintf(out, messages.CLILatestVersionDate+"\n", latestTag, latestPub)

	printServerVersion(client, out)
	err = compareVersions(latestTag, currentTag, out)
	return err
}

func isValidVersion(version string) bool {
	return version != ""
}

// printServerVersion outputs current server version
func printServerVersion(client houston.HoustonClientInterface, out io.Writer) {
	appCfg, err := client.GetAppConfig()
	if err != nil {
		fmt.Fprintf(out, messages.HoustonCurrentVersion+"\n", "Please authenticate to a cluster to see server version")
	}

	if appCfg != nil {
		fmt.Fprintf(out, messages.HoustonCurrentVersion+"\n", appCfg.Version)
	}
}
