package git

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

const MaxCommitMessageLineLength = 72

// HasUncommittedChanges checks repository for uncommitted changes
func HasUncommittedChanges(path string) bool {
	if !IsGitRepository(path) {
		return false
	}

	_, err := runGitCommand(path, []string{"diff", "--quiet", "HEAD"})
	return err != nil
}

// IsGitRepository checks if current directory is a git repository
func IsGitRepository(path string) bool {
	_, err := runGitCommand(path, []string{"rev-parse", "--is-inside-working-tree"})
	return err == nil
}

func GetRemoteRepository(path, remote string) (*url.URL, error) {
	urlStr, err := runGitCommand(path, []string{"remote", "get-url", remote})
	if err != nil {
		return nil, err
	}
	return parseGitURL(urlStr)
}

func GetLocalRepositoryPathPrefix(path, dir string) (string, error) {
	path, err := runGitCommand(path, []string{"-C", dir, "rev-parse", "--show-prefix"})
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(path, "/"), nil
}

func GetBranch(path string) (string, error) {
	return runGitCommand(path, []string{"rev-parse", "--abbrev-ref", "HEAD"})
}

func GetHeadCommit(path string) (sha, message, name, email string, err error) {
	commitJSON, err := runGitCommand(path, []string{"log", "-1", `--pretty=format:{"sha":"%H","name":"%an","email":"%ae"}`, "HEAD"})
	if err != nil {
		return "", "", "", "", err
	}

	// parse the commit JSON
	// e.g. {"sha":"c1b4c1b...","name":"author name","email":"author email"}
	commit := struct {
		Sha   string `json:"sha"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}{}
	err = json.Unmarshal([]byte(commitJSON), &commit)
	if err != nil {
		return "", "", "", "", err
	}

	// get the commit message, truncated to the first line with a max line length
	message, err = runGitCommand(path, []string{"log", "-1", "--pretty=%B", "HEAD"})
	if err != nil {
		return "", "", "", "", err
	}
	message = strings.Split(message, "\n")[0]
	if len(message) > MaxCommitMessageLineLength {
		message = message[:MaxCommitMessageLineLength]
	}

	return commit.Sha, message, commit.Name, commit.Email, nil
}

func runGitCommand(path string, args []string) (string, error) {
	if path != "" {
		args = append([]string{"-C", path}, args...)
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func parseGitURL(urlStr string) (*url.URL, error) {
	var parsedURL *url.URL
	var err error

	if strings.Contains(urlStr, "://") {
		parsedURL, err = url.Parse(urlStr)
		if err != nil {
			return nil, err
		}
	} else {
		// if the URL is not a full URL, assume it is an "SCP-like" SSH URL
		// e.g. git@github.com:astronomer/astro-cli
		parts := strings.Split(urlStr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid Git URL: %s", urlStr)
		}
		userHost := strings.Split(parts[0], "@")
		if len(userHost) > 2 {
			return nil, fmt.Errorf("invalid user@host format")
		}
		parsedURL = &url.URL{
			Scheme: "ssh",
			Host:   userHost[len(userHost)-1],
			Path:   parts[1],
		}
	}

	parsedURL.Path = strings.TrimSuffix(parsedURL.Path, ".git")

	return parsedURL, nil
}
