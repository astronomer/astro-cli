package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InWorkTree reports whether dir appears to be inside a git work tree: a .git
// directory or file (a submodule's, a worktree's) at dir or above it. It asks
// the file system, not git, so it answers even where git cannot.
func InWorkTree(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for d := abs; ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}

// CheckIgnored returns those of paths (relative to dir) that the .gitignore
// rules of the repository holding dir ignore, in the order git reports them.
// Tracked files are never reported, since git does not apply ignore rules to
// them.
//
// An error means git could not answer: it is not installed, refuses the
// repository (an unsafe owner, outside safe.directory), or fails on a path
// (one inside a submodule, say), with what git said. The caller decides what
// an unanswered question means; outside a git work tree there is no question,
// which InWorkTree tells first.
func CheckIgnored(dir string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree").CombinedOutput(); err != nil {
		return nil, gitError(err, out)
	}
	// NUL-separated both ways, so no file name can split or join an entry.
	cmd := exec.Command("git", "-C", dir, "check-ignore", "-z", "--stdin")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		// Exit 1 is an answer: nothing is ignored.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 && stderr.Len() == 0 {
			return nil, nil
		}
		return nil, gitError(err, stderr.Bytes())
	}
	var ignored []string
	for _, p := range strings.Split(out.String(), "\x00") {
		if p != "" {
			ignored = append(ignored, p)
		}
	}
	return ignored, nil
}

// gitError is err with the first line git wrote, which names the reason.
func gitError(err error, out []byte) error {
	msg, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if msg == "" {
		return fmt.Errorf("git: %w", err)
	}
	return fmt.Errorf("git: %s", msg)
}
