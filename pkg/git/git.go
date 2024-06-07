package git

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

var Path = ""

// HasUncommittedChanges checks repository for uncommitted changes
func HasUncommittedChanges() bool {
	if !IsGitRepository() {
		return false
	}

	_, err := runGitCommand("diff", "--quiet", "HEAD")
	return err != nil
}

// IsGitRepository checks if current directory is a git repository
func IsGitRepository() bool {
	_, err := runGitCommand("rev-parse", "--is-inside-working-tree")
	return err == nil
}

func GetRemoteRepository(remote string) (host, account, repo string, err error) {
	remoteURLStr, err := runGitCommand("remote", "get-url", remote)
	if err != nil {
		return "", "", "", err
	}
	remoteURL, err := url.Parse(remoteURLStr)
	if err != nil {
		return "", "", "", err
	}

	host = remoteURL.Hostname()
	accountRepo := strings.Split(remoteURL.Path, "/")
	if len(accountRepo) != 3 || accountRepo[0] != "" {
		return "", "", "", fmt.Errorf("failed to parse remote URL: %s", remoteURLStr)
	}
	account = accountRepo[1]
	repo = strings.TrimSuffix(accountRepo[2], ".git")

	return host, account, repo, nil
}

func GetLocalRepositoryPathPrefix(dir string) (string, error) {
	path, err := runGitCommand("-C", dir, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(path, "/"), nil
}

func GetBranch() (string, error) {
	return runGitCommand("rev-parse", "--abbrev-ref", "HEAD")
}

func GetHeadCommitSHA() (string, error) {
	return runGitCommand("rev-parse", "HEAD")
}

func GetHeadCommitAuthor() (name, email string, err error) {
	authorJSON, err := runGitCommand("log", "-1", `--pretty=format:{"name":"%an","email":"%ae"}`, "HEAD")
	if err != nil {
		return "", "", err
	}

	// parse the author JSON
	// e.g. {"name":"Jane Doe","email":"jane@doe.com"}
	author := struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}{}
	err = json.Unmarshal([]byte(authorJSON), &author)
	if err != nil {
		return "", "", err
	}

	return author.Name, author.Email, nil
}

func GetCommitURL(host, account, repo, sha string) (string, error) {
	switch host {
	case "github.com":
		return fmt.Sprintf("https://%s/%s/%s/commit/%s", host, account, repo, sha), nil
	default:
		return "", fmt.Errorf("unsupported Git provider: %s", host)
	}
}

func runGitCommand(args ...string) (string, error) {
	if Path != "" {
		args = append([]string{"-C", Path}, args...)
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
