package git

import (
	"os/exec"
)

// IsGitRepository checks if current directory is a git respository
func IsGitRepository() bool {
	_, err := exec.Command("git", "rev-parse", "--is-inside-working-tree").Output()
	if err != nil {
		return false
	}

	return true

}

// HasUncommitedChanges checks repository for uncommited changes
func HasUncommitedChanges() bool {
	if IsGitRepository() {
		// There is no system indepdendent way to get exit code
		// so we treat all exit codes the same, this works as long as we check IsGitRepository first
		_, err := exec.Command("git", "diff", "--quiet", "HEAD").Output()
		if err != nil {
			return true
		}
	}

	return false
}
