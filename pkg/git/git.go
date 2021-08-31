package git

import (
	"os/exec"
)

// IsGitRepository checks if current directory is a git repository
func IsGitRepository() bool {
	_, err := exec.Command("git", "rev-parse", "--is-inside-working-tree").Output()
	if err != nil {
		return false
	}

	return true
}

// HasUncommittedChanges checks repository for uncommitted changes
func HasUncommittedChanges() bool {
	if IsGitRepository() {
		// There is no system independent way to get exit code
		// so we treat all exit codes the same, this works as long as we check IsGitRepository first
		_, err := exec.Command("git", "diff", "--quiet", "HEAD").Output()
		if err != nil {
			return true
		}
	}

	return false
}
